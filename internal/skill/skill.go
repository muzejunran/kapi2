package skill

import (
	"context"
)

// AgentContext represents the current agent context
type AgentContext struct {
	UserID      string
	PageContext string
	Message     string
	State       map[string]interface{}
}

// SkillRequest represents a request to a skill
type SkillRequest struct {
	Context  AgentContext
	Intent   string
	Input    string      // 用户输入
	Args     map[string]interface{}
}

// Skill represents a high-level capability that may contain multiple tools
type Skill interface {
	// Initialize sets up the skill with configuration
	Initialize(config map[string]interface{}) error

	// GetID returns the skill ID
	GetID() string

	// CanHandle evaluates if this skill can handle the current context
	// Returns 0-1 where 1 is a perfect match
	CanHandle(context AgentContext) float64

	// GetTools returns the tools this skill provides
	GetTools() []Tool

	// Execute handles a skill request, may call multiple tools internally
	Execute(ctx context.Context, request SkillRequest) (*SkillResponse, error)

	// Cleanup releases any resources
	Cleanup() error

	// GetExtractionSchema returns the JSON schema for parameter extraction from user input
	GetExtractionSchema(ctx AgentContext) string
}

// BaseSkill provides common functionality for skills
type BaseSkill struct {
	ID          string
	Name        string
	Description string
	Version     string
	Tools       map[string]Tool
}

// NewBaseSkill creates a new base skill
func NewBaseSkill(id, name, description string) *BaseSkill {
	return &BaseSkill{
		ID:          id,
		Name:        name,
		Description: description,
		Version:     "1.0.0",
		Tools:       make(map[string]Tool),
	}
}

// RegisterTool registers a tool within this skill
func (bs *BaseSkill) RegisterTool(tool Tool) {
	bs.Tools[tool.ID] = tool
}

// GetTool retrieves a tool by ID
func (bs *BaseSkill) GetTool(id string) (Tool, bool) {
	tool, exists := bs.Tools[id]
	return tool, exists
}

// GetTools returns all tools
func (bs *BaseSkill) GetTools() []Tool {
	tools := make([]Tool, 0, len(bs.Tools))
	for _, tool := range bs.Tools {
		tools = append(tools, tool)
	}
	return tools
}

// Cleanup is a no-op by default
func (bs *BaseSkill) Cleanup() error {
	return nil
}

// GetID returns the skill ID
func (bs *BaseSkill) GetID() string {
	return bs.ID
}

// GetExtractionSchema returns default extraction schema
func (bs *BaseSkill) GetExtractionSchema(ctx AgentContext) string {
	return ""
}
