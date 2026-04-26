package skillserver

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"strings"
)

// PageIndex maps page names to allowed skill IDs.
// The special key "*" lists skills available on all pages.
type PageIndex map[string][]string

// LoadSkills loads all skill JSON configs from configFS/dir, skipping pages.json.
func LoadSkills(configFS fs.ReadDirFS, dir string) ([]SkillConfig, map[string]toolEntry, error) {
	entries, err := configFS.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read configs dir: %w", err)
	}

	var skills []SkillConfig
	toolIndex := make(map[string]toolEntry)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || entry.Name() == "pages.json" {
			continue
		}

		data, err := fs.ReadFile(configFS, dir+"/"+entry.Name())
		if err != nil {
			log.Printf("warn: skip %s: %v", entry.Name(), err)
			continue
		}

		var cfg SkillConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			log.Printf("warn: parse %s failed: %v", entry.Name(), err)
			continue
		}

		skills = append(skills, cfg)
		for _, tool := range cfg.Tools {
			toolIndex[tool.Name] = toolEntry{Skill: cfg, Tool: tool}
		}
		log.Printf("loaded skill: %s (layer=%s, tools=%d)", cfg.ID, cfg.Layer, len(cfg.Tools))
	}

	return skills, toolIndex, nil
}

// LoadPages loads the page→skill mapping from configFS/dir/pages.json.
func LoadPages(configFS fs.ReadDirFS, dir string) (PageIndex, error) {
	data, err := fs.ReadFile(configFS, dir+"/pages.json")
	if err != nil {
		return nil, fmt.Errorf("read pages.json: %w", err)
	}
	var index PageIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("parse pages.json: %w", err)
	}
	return index, nil
}

// toolEntry is the internal index entry for fast tool lookup.
type toolEntry struct {
	Skill SkillConfig
	Tool  ToolSpec
}

// MatchesPage reports whether skillID is available on pageContext.
// Skills listed under the "*" key in pageIndex are available on all pages.
func MatchesPage(pageIndex PageIndex, skillID, pageContext string) bool {
	for _, id := range pageIndex["*"] {
		if id == skillID {
			return true
		}
	}
	for _, id := range pageIndex[pageContext] {
		if id == skillID {
			return true
		}
	}
	return false
}
