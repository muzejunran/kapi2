package skill

import (
	"fmt"
	"sync"
)

// SkillRegistry manages all available skills
type SkillRegistry struct {
	skills      map[string]Skill
	mu          sync.RWMutex
	versionMgr  *SkillVersionManager
	// TODO: Add FileWatcher support when implementing hot reload
	// fileWatcher *FileWatcher
	// skillPaths  map[string]string
}

// NewSkillRegistry creates a new skill registry with version management
func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{
		skills:     make(map[string]Skill),
		versionMgr: NewSkillVersionManager(),
	}
}

// GetVersionManager returns the version manager
func (sr *SkillRegistry) GetVersionManager() *SkillVersionManager {
	return sr.versionMgr
}

// RegisterSkill registers a new skill
func (sr *SkillRegistry) RegisterSkill(skill Skill) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if skill == nil {
		return fmt.Errorf("skill cannot be nil")
	}

	// Initialize the skill
	if err := skill.Initialize(nil); err != nil {
		return fmt.Errorf("failed to initialize skill: %w", err)
	}

	sr.skills[skill.GetID()] = skill
	return nil
}

// UpdateSkill updates an existing skill (for hot reload)
func (sr *SkillRegistry) UpdateSkill(skill Skill) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if skill == nil {
		return fmt.Errorf("skill cannot be nil")
	}

	// Cleanup old skill if exists
	if oldSkill, exists := sr.skills[skill.GetID()]; exists {
		oldSkill.Cleanup()
	}

	// Initialize the new skill
	if err := skill.Initialize(nil); err != nil {
		return fmt.Errorf("failed to initialize skill: %w", err)
	}

	sr.skills[skill.GetID()] = skill
	return nil
}

// GetSkill retrieves a skill by ID
func (sr *SkillRegistry) GetSkill(id string) (Skill, bool) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	skill, exists := sr.skills[id]
	return skill, exists
}

// GetAllSkills returns all registered skills
func (sr *SkillRegistry) GetAllSkills() []Skill {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	skills := make([]Skill, 0, len(sr.skills))
	for _, skill := range sr.skills {
		skills = append(skills, skill)
	}
	return skills
}

// GetBestSkillForContext returns the skill best suited for the current context
func (sr *SkillRegistry) GetBestSkillForContext(ctx AgentContext) (Skill, float64) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	var bestSkill Skill
	var bestScore float64

	for _, skill := range sr.skills {
		score := skill.CanHandle(ctx)
		if score > bestScore {
			bestScore = score
			bestSkill = skill
		}
	}

	return bestSkill, bestScore
}

// GetToolsForContext returns all tools from skills that can handle the context
func (sr *SkillRegistry) GetToolsForContext(ctx AgentContext) []Tool {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	tools := make([]Tool, 0)

	for _, skill := range sr.skills {
		// Only include tools from skills that can handle this context
		if skill.CanHandle(ctx) > 0 {
			tools = append(tools, skill.GetTools()...)
		}
	}

	return tools
}

// RemoveSkill removes a skill from the registry
func (sr *SkillRegistry) RemoveSkill(id string) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	skill, exists := sr.skills[id]
	if !exists {
		return fmt.Errorf("skill not found: %s", id)
	}

	// Cleanup the skill
	if err := skill.Cleanup(); err != nil {
		return fmt.Errorf("failed to cleanup skill: %w", err)
	}

	delete(sr.skills, id)
	return nil
}

// Clear removes all skills
func (sr *SkillRegistry) Clear() error {
	sr.mu.Lock()
	defer sr.mu.RUnlock()

	for _, skill := range sr.skills {
		if err := skill.Cleanup(); err != nil {
			return fmt.Errorf("failed to cleanup skill: %w", err)
		}
	}

	sr.skills = make(map[string]Skill)
	return nil
}

// Count returns the number of registered skills
func (sr *SkillRegistry) Count() int {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	return len(sr.skills)
}

// Shutdown cleanly shuts down all skills
func (sr *SkillRegistry) Shutdown() error {
	return sr.Clear()
}
