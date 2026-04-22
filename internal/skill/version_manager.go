package skill

import (
	"encoding/json"
	"fmt"
	"time"
)

// SkillVersion represents a version of a skill
type SkillVersion struct {
	Version     string                 `json:"version"`
	Status      SkillStatus            `json:"status"`
	Config      *SkillConfig           `json:"config"`
	DeployTime  time.Time              `json:"deploy_time"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// SkillStatus represents the deployment status
type SkillStatus string

const (
	StatusActive      SkillStatus = "active"
	StatusGray        SkillStatus = "gray"
	StatusInactive   SkillStatus = "inactive"
	StatusFailed      SkillStatus = "failed"
)

// GrayDeploymentConfig contains gray deployment configuration
type GrayDeploymentConfig struct {
	Enabled       bool             `json:"enabled"`
	Strategy      string           `json:"strategy"` // "percentage", "user_segment", "page"
	Percentage    float64          `json:"percentage"`
	UserSegments  []string         `json:"user_segments"`
	PageMap       map[string]float64 `json:"page_map"`
	CanaryVersion string          `json:"canary_version"`
}

// SkillVersionManager manages skill versions
type SkillVersionManager struct {
	versions map[string][]*SkillVersion
}

// NewSkillVersionManager creates a new skill version manager
func NewSkillVersionManager() *SkillVersionManager {
	return &SkillVersionManager{
		versions: make(map[string][]*SkillVersion),
	}
}

// AddVersion adds a new version for a skill
func (svm *SkillVersionManager) AddVersion(skillID string, version *SkillVersion) {
	svm.versions[skillID] = append(svm.versions[skillID], version)
}

// GetActiveVersion gets the active version for a skill
func (svm *SkillVersionManager) GetActiveVersion(skillID string) (*SkillVersion, bool) {
	versions, exists := svm.versions[skillID]
	if !exists || len(versions) == 0 {
		return nil, false
	}

	// Find the latest active version
	for i := len(versions) - 1; i >= 0; i-- {
		if versions[i].Status == StatusActive {
			return versions[i], true
		}
	}
	return nil, false
}

// GetVersion gets a specific version
func (svm *SkillVersionManager) GetVersion(skillID, version string) (*SkillVersion, bool) {
	versions, exists := svm.versions[skillID]
	if !exists {
		return nil, false
	}

	for _, v := range versions {
		if v.Version == version {
			return v, true
		}
	}
	return nil, false
}

// Rollback rolls back a skill to a specific version
func (svm *SkillVersionManager) Rollback(skillID, version string) error {
	versions, exists := svm.versions[skillID]
	if !exists {
		return fmt.Errorf("skill %s not found", skillID)
	}

	// Deactivate current active version
	if active, ok := svm.GetActiveVersion(skillID); ok {
		active.Status = StatusInactive
	}

	// Activate the specified version
	for _, v := range versions {
		if v.Version == version {
			v.Status = StatusActive
			return nil
		}
	}

	return fmt.Errorf("version %s not found for skill %s", version, skillID)
}

// GetGrayConfig gets gray deployment config for a skill
func (svm *SkillVersionManager) GetGrayConfig(skillID string) *GrayDeploymentConfig {
	if active, ok := svm.GetActiveVersion(skillID); ok {
		if config := active.Metadata["gray_config"]; config != nil {
			if grayConfig, ok := config.(map[string]interface{}); ok {
				var gdc GrayDeploymentConfig
				b, _ := json.Marshal(grayConfig)
				json.Unmarshal(b, &gdc)
				return &gdc
			}
		}
	}
	return nil
}

// ShouldUseCanary determines if a request should use canary version
func (svm *SkillVersionManager) ShouldUseCanary(skillID string, userID string, page string) bool {
	grayConfig := svm.GetGrayConfig(skillID)
	if grayConfig == nil || !grayConfig.Enabled {
		return false
	}

	switch grayConfig.Strategy {
	case "percentage":
		// Simple hash-based percentage for demo
		hash := uint64(0)
		for _, c := range userID+page {
			hash = hash*31 + uint64(c)
		}
		return hash%100 < uint64(grayConfig.Percentage*100)

	case "user_segment":
		for _, segment := range grayConfig.UserSegments {
			if userID == segment {
				return true
			}
		}
		return false

	case "page":
		if percentage, ok := grayConfig.PageMap[page]; ok {
			hash := uint64(0)
			for _, c := range userID {
				hash = hash*31 + uint64(c)
			}
			return hash%100 < uint64(percentage*100)
		}
		return false

	default:
		return false
	}
}

// GetCanaryVersion gets the canary version for a skill
func (svm *SkillVersionManager) GetCanaryVersion(skillID string) (*SkillVersion, bool) {
	grayConfig := svm.GetGrayConfig(skillID)
	if grayConfig == nil {
		return nil, false
	}

	versions, exists := svm.versions[skillID]
	if !exists {
		return nil, false
	}

	for _, v := range versions {
		if v.Version == grayConfig.CanaryVersion {
			return v, true
		}
	}
	return nil, false
}