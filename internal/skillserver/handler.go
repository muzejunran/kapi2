package skillserver

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"text/template"

	"ai-assistant-service/internal/logger"
	"github.com/sirupsen/logrus"
)

// Server HTTP 服务，持有 skill 配置和执行器
type Server struct {
	skills    []SkillConfig
	toolIndex map[string]toolEntry
	executor  *Executor
}

// NewServer 创建服务，configFS 为嵌入的配置文件系统
func NewServer(configFS fs.ReadDirFS, configDir string, executor *Executor) (*Server, error) {
	skills, toolIndex, err := LoadSkills(configFS, configDir)
	if err != nil {
		return nil, err
	}
	return &Server{
		skills:    skills,
		toolIndex: toolIndex,
		executor:  executor,
	}, nil
}

// RegisterRoutes 注册所有路由
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/skills", s.handleGetSkills)
	mux.HandleFunc("/execute", s.handleExecute)
	mux.HandleFunc("/health", s.handleHealth)
}

// handleGetSkills GET /skills?page_context=xxx
func (s *Server) handleGetSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	log := logger.FromContext(ctx)
	pageContext := r.URL.Query().Get("page_context")

	var tools []OpenAITool
	for _, skill := range s.skills {
		// empty page_context means no filtering — return all skills
		if pageContext != "" && !MatchesPage(skill.SupportedPages, pageContext) {
			continue
		}
		for _, t := range skill.Tools {
			tools = append(tools, OpenAITool{
				Type: "function",
				Function: OpenAIFunction{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.Parameters,
				},
			})
		}
	}
	if tools == nil {
		tools = []OpenAITool{}
	}

	log.WithFields(logrus.Fields{
		"page_context": pageContext,
		"tool_count":   len(tools),
	}).Info("[skill-server] GET /skills")

	writeJSON(w, GetSkillsResponse{Tools: tools})
}

// handleExecute POST /execute
func (s *Server) handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	log := logger.FromContext(ctx)

	var req ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, ExecuteResponse{
			Success:    false,
			ActionType: ActionReturnDirect,
			Error:      "invalid request body",
		})
		return
	}

	log.WithFields(logrus.Fields{
		"tool":    req.ToolName,
		"user_id": req.UserID,
	}).Info("[skill-server] POST /execute start")

	entry, ok := s.toolIndex[req.ToolName]
	if !ok {
		log.WithField("tool", req.ToolName).Warn("[skill-server] unknown tool")
		writeJSON(w, ExecuteResponse{
			Success:    false,
			ActionType: ActionReturnDirect,
			Error:      "unknown tool: " + req.ToolName,
		})
		return
	}

	result, err := s.executor.Execute(ctx, req.ToolName, req.UserID, req.Args)
	if err != nil {
		log.WithFields(logrus.Fields{
			"tool":  req.ToolName,
			"error": err.Error(),
		}).Error("[skill-server] tool execution failed")
		writeJSON(w, ExecuteResponse{
			Success:    false,
			ActionType: entry.Tool.ActionType,
			Error:      err.Error(),
		})
		return
	}

	resp := ExecuteResponse{
		Success:    true,
		Result:     result,
		ActionType: entry.Tool.ActionType,
		NextTool:   entry.Tool.NextTool,
	}

	if entry.Tool.ActionType == ActionReturnDirect && entry.Tool.ReturnTemplate != "" {
		resp.Message = renderTemplate(entry.Tool.ReturnTemplate, result)
	}

	log.WithFields(logrus.Fields{
		"tool":        req.ToolName,
		"action_type": resp.ActionType,
	}).Info("[skill-server] POST /execute done")

	writeJSON(w, resp)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

// ── 工具函数 ──────────────────────────────────────────────────────────────────

func renderTemplate(tmpl string, data map[string]interface{}) string {
	t, err := template.New("").Parse(tmpl)
	if err != nil {
		return tmpl
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return tmpl
	}
	return buf.String()
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger.New().WithError(err).Error("writeJSON error")
	}
}
