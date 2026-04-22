package skill

import (
	"fmt"
	"sync"
)

// ToolRegistry manages all available tools
type ToolRegistry struct {
	tools map[string]Tool
	mu    sync.RWMutex
}

// NewToolRegistry creates a new tool registry
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

// RegisterTool registers a new tool
func (tr *ToolRegistry) RegisterTool(tool Tool) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if tool.ID == "" {
		return fmt.Errorf("tool ID is required")
	}

	if tool.Name == "" {
		return fmt.Errorf("tool name is required")
	}

	tr.tools[tool.ID] = tool
	return nil
}

// RegisterTools registers multiple tools
func (tr *ToolRegistry) RegisterTools(tools []Tool) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	for _, tool := range tools {
		if tool.ID == "" {
			return fmt.Errorf("tool ID is required")
		}
		if tool.Name == "" {
			return fmt.Errorf("tool name is required for tool %s", tool.ID)
		}
		tr.tools[tool.ID] = tool
	}
	return nil
}

// GetTool retrieves a tool by ID
func (tr *ToolRegistry) GetTool(id string) (Tool, bool) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	tool, exists := tr.tools[id]
	return tool, exists
}

// GetAllTools returns all registered tools
func (tr *ToolRegistry) GetAllTools() []Tool {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	tools := make([]Tool, 0, len(tr.tools))
	for _, tool := range tr.tools {
		tools = append(tools, tool)
	}
	return tools
}

// GetToolsForPage returns tools that are available for a specific page
// In this simple version, all tools are available. In the future,
// tools can have page restrictions.
func (tr *ToolRegistry) GetToolsForPage(pageContext string) []Tool {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	// For now, return all tools
	// In the future, tools could have page restrictions
	tools := make([]Tool, 0, len(tr.tools))
	for _, tool := range tr.tools {
		tools = append(tools, tool)
	}
	return tools
}

// RemoveTool removes a tool from the registry
func (tr *ToolRegistry) RemoveTool(id string) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if _, exists := tr.tools[id]; !exists {
		return fmt.Errorf("tool not found: %s", id)
	}

	delete(tr.tools, id)
	return nil
}

// Clear removes all tools
func (tr *ToolRegistry) Clear() {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	tr.tools = make(map[string]Tool)
}

// Count returns the number of registered tools
func (tr *ToolRegistry) Count() int {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	return len(tr.tools)
}
