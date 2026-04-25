package skillserver

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"strings"
)

// LoadSkills 从嵌入的 FS 加载所有 skill 配置，返回 skill 列表和 tool 索引
func LoadSkills(configFS fs.ReadDirFS, dir string) ([]SkillConfig, map[string]toolEntry, error) {
	entries, err := configFS.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read configs dir: %w", err)
	}

	var skills []SkillConfig
	toolIndex := make(map[string]toolEntry)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
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
		log.Printf("loaded skill: %s (layer=%s, pages=%v, tools=%d)",
			cfg.ID, cfg.Layer, cfg.SupportedPages, len(cfg.Tools))
	}

	return skills, toolIndex, nil
}

// toolEntry 内部索引项，用于 tool 快速查找
type toolEntry struct {
	Skill SkillConfig
	Tool  ToolSpec
}

// MatchesPage 判断 skill 是否对当前页面可用
func MatchesPage(supportedPages []string, pageContext string) bool {
	if len(supportedPages) == 0 {
		return true // 全局 skill，所有页面可用
	}
	for _, p := range supportedPages {
		if p == "*" || p == pageContext {
			return true
		}
	}
	return false
}
