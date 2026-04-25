package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-assistant-service/internal/logger"
	"github.com/sirupsen/logrus"
)

// MemoryDeterminer uses an LLM to extract memory signals from conversation turns
type MemoryDeterminer struct {
	endpoint  string
	apiKey    string
	modelName string
	client    *http.Client
	logger    *logrus.Logger
}

type determineResult struct {
	ProfileUpdate    *string  `json:"profile_update"`
	PreferenceUpdate *string  `json:"preference_update"`
	NewFacts         []string `json:"new_facts"`
	Summary          string   `json:"summary"`
}

// NewMemoryDeterminer creates a new memory determiner
func NewMemoryDeterminer(endpoint, apiKey, modelName string) *MemoryDeterminer {
	return &MemoryDeterminer{
		endpoint:  endpoint,
		apiKey:    apiKey,
		modelName: modelName,
		client:    &http.Client{Timeout: 90 * time.Second},
		logger:    logger.New(),
	}
}

const determinerSystemPrompt = `你是一个用户画像提取助手。从对话中提取用户的个人信息、偏好和关键事实。
只提取明确提到的信息，不要推测。
返回严格JSON（无markdown代码块，所有值必须是字符串或数组，不能是嵌套对象）:
{"profile_update": null或"一句话描述用户画像", "preference_update": null或"一句话描述用户偏好", "new_facts": ["事实1","事实2"], "summary": "一句话摘要"}
示例: {"profile_update": "程序员，正在找工作", "preference_update": null, "new_facts": [], "summary": "用户是程序员"}`

// llmMessage holds both regular and reasoning-model content fields.
type llmMessage struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
}

// llmChatResponse is the non-streaming response envelope.
type llmChatResponse struct {
	Choices []struct {
		Message llmMessage `json:"message"`
	} `json:"choices"`
}

// extractContent returns the usable text from an LLM response message,
// falling back to reasoning_content when content is empty (reasoning models).
// It also strips markdown code fences that some models insert despite instructions.
func extractContent(m llmMessage) string {
	text := strings.TrimSpace(m.Content)
	if text == "" {
		text = strings.TrimSpace(m.ReasoningContent)
	}
	return stripCodeFence(text)
}

// stripCodeFence removes ```json … ``` or ``` … ``` wrappers from LLM output.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Drop the opening fence line (```json or ```)
	if nl := strings.Index(s, "\n"); nl != -1 {
		s = s[nl+1:]
	}
	// Drop the closing fence
	if idx := strings.LastIndex(s, "```"); idx != -1 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

// Analyze extracts memory signals from a single conversation turn (non-streaming)
func (d *MemoryDeterminer) Analyze(ctx context.Context, userMsg, assistantMsg string) (*determineResult, error) {
	conversation := fmt.Sprintf("用户: %s\n助手: %s", userMsg, assistantMsg)

	reqBody, err := json.Marshal(map[string]interface{}{
		"model": d.modelName,
		"messages": []map[string]interface{}{
			{"role": "system", "content": determinerSystemPrompt},
			{"role": "user", "content": conversation},
		},
		"max_tokens":  1500,
		"temperature": 0.1,
		"stream":      false,
	})
	if err != nil {
		return nil, err
	}

	d.logger.WithFields(logrus.Fields{
		"model":       d.modelName,
		"user_len":    len(userMsg),
		"assistant_len": len(assistantMsg),
	}).Debug("[determiner] analyze request")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.apiKey)

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var llmResp llmChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&llmResp); err != nil {
		return nil, err
	}
	if len(llmResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in determiner response")
	}

	content := extractContent(llmResp.Choices[0].Message)
	d.logger.WithFields(logrus.Fields{
		"raw_content":       llmResp.Choices[0].Message.Content,
		"reasoning_content": llmResp.Choices[0].Message.ReasoningContent != "",
		"extracted":         content,
	}).Debug("[determiner] LLM response received")

	if content == "" {
		return nil, fmt.Errorf("determiner returned empty content")
	}

	var result determineResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		d.logger.WithFields(logrus.Fields{
			"content": content,
			"error":   err.Error(),
		}).Warn("[determiner] failed to parse JSON, using raw as summary")
		result = determineResult{Summary: content}
	}
	return &result, nil
}

// compressSummary compresses a long summary into a shorter one via LLM
func (d *MemoryDeterminer) compressSummary(ctx context.Context, summary string) (string, error) {
	if CalculateTokens(summary) <= maxSummaryTokens {
		return summary, nil
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"model": d.modelName,
		"messages": []map[string]interface{}{
			{"role": "system", "content": "请将以下对话摘要压缩为简短的一两句话，保留最重要的信息。只返回压缩后的文本，不要其他内容。"},
			{"role": "user", "content": summary},
		},
		"max_tokens":  200,
		"temperature": 0.1,
		"stream":      false,
	})
	if err != nil {
		return summary, err
	}

	d.logger.WithFields(logrus.Fields{
		"model":       d.modelName,
		"summary_len": len(summary),
	}).Debug("[determiner] compress request")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return summary, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.apiKey)

	resp, err := d.client.Do(req)
	if err != nil {
		return summary, err
	}
	defer resp.Body.Close()

	var llmResp llmChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&llmResp); err != nil {
		return summary, err
	}
	if len(llmResp.Choices) == 0 {
		return summary, nil
	}
	compressed := extractContent(llmResp.Choices[0].Message)
	if compressed == "" {
		return summary, nil
	}
	return compressed, nil
}
