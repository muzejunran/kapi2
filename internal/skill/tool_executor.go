package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// ToolExecutor executes tools with timeout and error handling
type ToolExecutor struct {
	timeout time.Duration
	logger  *logrus.Logger
}

// NewToolExecutor creates a new tool executor
func NewToolExecutor(timeout time.Duration) *ToolExecutor {
	return &ToolExecutor{
		timeout: timeout,
		logger:  logrus.New(),
	}
}

// Execute executes a tool with given arguments
func (te *ToolExecutor) Execute(ctx context.Context, tool Tool, args map[string]interface{}) (interface{}, error) {
	// Validate input parameters if they are defined
	if len(tool.Parameters) > 0 {
		if err := te.validateParameters(tool.Parameters, args); err != nil {
			return nil, fmt.Errorf("invalid parameters: %w", err)
		}
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(ctx, te.timeout)
	defer cancel()

	// Execute the tool handler
	if tool.Handler == nil {
		return nil, fmt.Errorf("tool handler not set: %s", tool.ID)
	}

	result, err := te.executeWithTimeout(ctx, tool.Handler, args)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// executeWithTimeout executes a tool handler with context timeout
func (te *ToolExecutor) executeWithTimeout(ctx context.Context, handler ToolHandler, args map[string]interface{}) (interface{}, error) {
	resultChan := make(chan interface{})
	errChan := make(chan error, 1)

	go func() {
		result, err := handler(args)
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- result
	}()

	select {
	case result := <-resultChan:
		return result, nil
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("tool execution timeout")
	}
}

// validateParameters validates input parameters against JSON schema
func (te *ToolExecutor) validateParameters(schema json.RawMessage, args map[string]interface{}) error {
	// Parse the JSON schema
	var schemaObj map[string]interface{}
	if err := json.Unmarshal(schema, &schemaObj); err != nil {
		return fmt.Errorf("invalid JSON schema: %w", err)
	}

	// Check if it's a proper JSON Schema
	_, ok := schemaObj["properties"].(map[string]interface{})
	if !ok {
		return nil // No properties to validate
	}

	required, ok := schemaObj["required"].([]interface{})
	if !ok {
		required = []interface{}{}
	}

	// Validate required fields
	for _, field := range required {
		fieldName, ok := field.(string)
		if !ok {
			continue
		}
		if _, exists := args[fieldName]; !exists {
			return fmt.Errorf("missing required parameter: %s", fieldName)
		}
	}

	return nil
}
