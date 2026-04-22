package skill

import (
	"encoding/json"
)

// Tool represents a single callable function (like OpenAI's Function/Tool)
type Tool struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  json.RawMessage        `json:"parameters"` // JSON Schema for parameters
	Handler     ToolHandler            `json:"-"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ToolHandler executes a tool with given arguments
type ToolHandler func(args map[string]interface{}) (interface{}, error)

// ToolResult represents the result of a tool execution
type ToolResult struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}
