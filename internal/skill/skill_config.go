package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"
)

// ========== 新的标准格式配置结构 ==========

// ToolDef 工具定义（OpenAI Function Calling 标准）
type ToolDef struct {
	Type     string       `json:"type"`
	Function FunctionDef  `json:"function"`
}

// FunctionDef 函数定义
type FunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// RuntimeConfig 运行时配置
type RuntimeConfig struct {
	DefaultTimeoutMs int                    `json:"default_timeout_ms"`
	MaxRetries       int                    `json:"max_retries"`
	ErrorMessage     string                 `json:"error_message"`
	DefaultParams    map[string]interface{} `json:"default_params"`
	FallbackTool     string                 `json:"fallback_tool"`
}

// SkillMetadata 技能元数据
type SkillMetadata struct {
	Author    string   `json:"author"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
	Tags      []string `json:"tags"`
	Priority  int      `json:"priority"`
}

// DeploymentConfig 部署配置
type DeploymentConfig struct {
	Enabled     bool             `json:"enabled"`
	GrayRelease GrayReleaseConfig `json:"gray_release"`
}

// GrayReleaseConfig 灰度发布配置
type GrayReleaseConfig struct {
	Enabled       bool    `json:"enabled"`
	Strategy      string  `json:"strategy"`
	Percentage    float64 `json:"percentage"`
	CanaryVersion string  `json:"canary_version"`
}

// NewSkillConfig 新的技能配置结构
type NewSkillConfig struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Description    string           `json:"description"`
	Version        string           `json:"version"`
	Enabled        bool             `json:"enabled"`
	SupportedPages []string         `json:"supported_pages"`
	Tools          []ToolDef        `json:"tools"`
	RuntimeConfig  RuntimeConfig    `json:"runtime_config"`
	Deployment     DeploymentConfig `json:"deployment"`
	Metadata       SkillMetadata    `json:"metadata"`
}

// ========== 旧格式配置结构（保持兼容）==========

// OperationSchema 定义操作的参数 Schema
type OperationSchema struct {
	Operation string                 `json:"operation"`
	Name      string                 `json:"name"`
	Params    json.RawMessage        `json:"params"`
	Required  []string               `json:"required"`
	Optional  map[string]interface{} `json:"optional"`
	NeedsLLM  bool                   `json:"needs_llm"`
}

// SkillConfig 旧的技能配置结构（保持兼容）
type SkillConfig struct {
	ID             string
	Name           string
	Description    string
	Version        string
	SupportedPages []string
	Operations     map[string]OperationSchema
	ErrorMessage   string
}

// ========== 通用类型 ==========

// ExtractionResult LLM 参数提取结果
type ExtractionResult struct {
	Operation string                 `json:"operation"`
	Params    map[string]interface{} `json:"params"`
	Success   bool                   `json:"success"`
	Error     string                 `json:"error,omitempty"`
	Confidence float64               `json:"confidence"`
}

// SkillResponse 技能执行响应
type SkillResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Context *Context    `json:"context,omitempty"`
}

// Context 上下文信息
type Context struct {
	Query  string `json:"query"`
	Answer string `json:"answer"`
}

// ========== 加载配置函数 ==========

// LoadSkillConfig 加载技能配置（支持新旧格式）
func LoadSkillConfig(filepath string) (*NewSkillConfig, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config NewSkillConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// 处理 runtime_config 中的模板变量
	if config.RuntimeConfig.DefaultParams != nil {
		config.RuntimeConfig.DefaultParams = processTemplateValues(config.RuntimeConfig.DefaultParams)
	}

	return &config, nil
}

// LoadOldSkillConfig 加载旧格式配置（保持兼容）
func LoadOldSkillConfig(filepath string) (*SkillConfig, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var tempConfig struct {
		ID             string          `json:"id"`
		Name           string          `json:"name"`
		Description    string          `json:"description"`
		Version        string          `json:"version"`
		SupportedPages []string        `json:"supported_pages"`
		ErrorMessage   string          `json:"error_message"`
		Operations     map[string]struct {
			Operation string                 `json:"operation"`
			Name      string                 `json:"name"`
			Params    json.RawMessage        `json:"params"`
			Required  []string               `json:"required"`
			Optional  map[string]interface{} `json:"optional"`
			NeedsLLM  bool                   `json:"needs_llm"`
		} `json:"operations"`
	}

	if err := json.Unmarshal(data, &tempConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// 处理模板值
	processedOps := make(map[string]OperationSchema)
	for opKey, op := range tempConfig.Operations {
		processedOps[opKey] = OperationSchema{
			Operation: op.Operation,
			Name:      op.Name,
			Params:    op.Params,
			Required:  op.Required,
			Optional:  processTemplateValues(op.Optional),
			NeedsLLM:  op.NeedsLLM,
		}
	}

	return &SkillConfig{
		ID:             tempConfig.ID,
		Name:           tempConfig.Name,
		Description:    tempConfig.Description,
		Version:        tempConfig.Version,
		SupportedPages: tempConfig.SupportedPages,
		ErrorMessage:   tempConfig.ErrorMessage,
		Operations:     processedOps,
	}, nil
}

// processTemplateValues 处理模板变量（如 {{.Today}}）
func processTemplateValues(values map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	today := time.Now().Format("2006-01-02")

	for k, v := range values {
		if str, ok := v.(string); ok && strings.Contains(str, "{{") {
			tmpl, err := template.New("value").Parse(str)
			if err == nil {
				var buf strings.Builder
				tmpl.Execute(&buf, map[string]string{"Today": today})
				result[k] = buf.String()
			} else {
				result[k] = v
			}
		} else {
			result[k] = v
		}
	}

	return result
}
