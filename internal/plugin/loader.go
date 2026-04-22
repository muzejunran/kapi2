package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	stdplugin "plugin"

	"github.com/sirupsen/logrus"
)

// Loader 插件加载器
type Loader struct {
	skillsDir   string               // 源码目录（.go, .json）
	binDir      string               // 编译输出目录（.so）
	deps        *Dependencies
	plugins     map[string]Plugin    // skillID -> plugin
	mu          sync.RWMutex
	logger      *logrus.Logger
	OnReload    func(skillID string, skill Skill) // Skill 重新加载时的回调
}

// Plugin 已加载的插件
type Plugin struct {
	Skill      Skill
	SourceFile string  // .go 源文件路径
	SoFile     string  // .so 文件路径
	ConfigFile string  // .json 配置文件路径
	BuildCmd   *exec.Cmd // 当前运行的编译命令
}

// NewLoader 创建插件加载器
func NewLoader(skillsDir, binDir string, deps *Dependencies, logger *logrus.Logger) *Loader {
	return &Loader{
		skillsDir: skillsDir,
		binDir:    binDir,
		deps:      deps,
		plugins:   make(map[string]Plugin),
		logger:    logger,
	}
}

// LoadAll 加载所有插件
func (l *Loader) LoadAll() error {
	// 如果 skillsDir 为空（生产模式），直接加载 .so 文件
	if l.skillsDir == "" {
		return l.loadPluginsFromBin()
	}

	// 确保输出目录存在
	if err := os.MkdirAll(l.binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin dir: %w", err)
	}

	// 扫描 skills/ 目录
	entries, err := os.ReadDir(l.skillsDir)
	if err != nil {
		return fmt.Errorf("failed to read skills dir: %w", err)
	}

	// 检查是否有 .go 文件（平铺结构）
	hasGoFiles := false
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".go" {
			hasGoFiles = true
			break
		}
	}

	// 如果有 .go 文件，使用平铺结构加载
	if hasGoFiles {
		return l.loadSkillsFlat(l.skillsDir)
	}

	// 否则使用子目录结构加载
	for _, entry := range entries {
		if entry.IsDir() {
			skillDir := filepath.Join(l.skillsDir, entry.Name())
			if err := l.loadSkillFromDir(skillDir); err != nil {
				l.logger.Warnf("Failed to load skill from %s: %v", skillDir, err)
			}
		}
	}

	return nil
}

// loadPluginsFromBin 从 binDir 直接加载 .so 文件（生产模式）
func (l *Loader) loadPluginsFromBin() error {
	soFiles, err := filepath.Glob(filepath.Join(l.binDir, "*.so"))
	if err != nil || len(soFiles) == 0 {
		return fmt.Errorf("no .so files found in %s", l.binDir)
	}

	for _, soFile := range soFiles {
		// 从文件名提取 skill ID
		skillID := strings.TrimSuffix(filepath.Base(soFile), ".so")

		// 加载插件
		newSkill, err := l.loadPlugin(soFile, skillID)
		if err != nil {
			l.logger.Warnf("Failed to load plugin %s: %v", skillID, err)
			continue
		}

		l.mu.Lock()
		// 清理旧插件（如果存在）
		if oldPlugin, exists := l.plugins[skillID]; exists && oldPlugin.Skill != nil {
			oldPlugin.Skill.Cleanup()
		}

		l.plugins[skillID] = Plugin{
			Skill:      newSkill,
			SourceFile: "",
			SoFile:     soFile,
			ConfigFile: "",
		}
		l.mu.Unlock()

		l.logger.Infof("✓ Loaded plugin: %s", skillID)
	}

	return nil
}

// loadSkillsFlat 从平铺目录加载技能
func (l *Loader) loadSkillsFlat(dir string) error {
	// 查找所有 .go 文件
	goFiles, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil || len(goFiles) == 0 {
		return fmt.Errorf("no .go files found in %s", dir)
	}

	// 遍历每个 .go 文件，查找对应的 .json 配置文件
	for _, goFile := range goFiles {
		baseName := filepath.Base(goFile)
		jsonFile := filepath.Join(dir, baseName[:len(baseName)-3]+".json")

		if _, err := os.Stat(jsonFile); os.IsNotExist(err) {
			l.logger.Warnf("No config file found for %s, skipping", goFile)
			continue
		}

		// 加载技能
		if err := l.loadSkillFromFiles(goFile, jsonFile); err != nil {
			l.logger.Warnf("Failed to load skill from %s: %v", goFile, err)
		}
	}

	return nil
}

// loadSkillFromFiles 从 .go 和 .json 文件加载单个技能
func (l *Loader) loadSkillFromFiles(goFile, jsonFile string) error {
	// 读取配置获取 skill ID
	skillID, err := l.getSkillIDFromConfig(jsonFile)
	if err != nil {
		return fmt.Errorf("failed to get skill ID from config: %w", err)
	}

	// 编译插件
	soFile := filepath.Join(l.binDir, skillID+".so")
	if err := l.buildSkill([]string{goFile}, soFile); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	// 加载插件
	newSkill, err := l.loadPlugin(soFile, skillID)
	if err != nil {
		return fmt.Errorf("load plugin failed: %w", err)
	}

	l.mu.Lock()
	// 清理旧插件（如果存在）
	if oldPlugin, exists := l.plugins[skillID]; exists && oldPlugin.Skill != nil {
		oldPlugin.Skill.Cleanup()
	}

	l.plugins[skillID] = Plugin{
		Skill:      newSkill,
		SourceFile: goFile,
		SoFile:     soFile,
		ConfigFile: jsonFile,
	}
	l.mu.Unlock()

	l.logger.Infof("✓ Loaded plugin: %s", skillID)
	return nil
}

// loadSkillFromDir 从目录加载单个技能
func (l *Loader) loadSkillFromDir(skillDir string) error {
	// 查找 .json 配置文件
	jsonFiles, err := filepath.Glob(filepath.Join(skillDir, "*.json"))
	if err != nil || len(jsonFiles) == 0 {
		return fmt.Errorf("no .json config found in %s", skillDir)
	}
	configFile := jsonFiles[0]

	// 读取配置获取 skill ID
	skillID, err := l.getSkillIDFromConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to get skill ID from config: %w", err)
	}

	// 查找 .go 源文件
	goFiles, err := filepath.Glob(filepath.Join(skillDir, "*.go"))
	soFile := filepath.Join(l.binDir, skillID+".so")

	// 如果有 .go 文件，编译插件
	if err == nil && len(goFiles) > 0 {
		if err := l.buildSkill(goFiles, soFile); err != nil {
			return fmt.Errorf("build failed: %w", err)
		}
	} else {
		// 没有 .go 文件，检查 .so 是否存在
		if _, err := os.Stat(soFile); os.IsNotExist(err) {
			return fmt.Errorf("no .go files found and .so not exists: %s", skillDir)
		}
	}

	// 加载插件
	newSkill, err := l.loadPlugin(soFile, skillID)
	if err != nil {
		return fmt.Errorf("load plugin failed: %w", err)
	}

	l.mu.Lock()
	// 清理旧插件（如果存在）
	if oldPlugin, exists := l.plugins[skillID]; exists && oldPlugin.Skill != nil {
		oldPlugin.Skill.Cleanup()
	}

	l.plugins[skillID] = Plugin{
		Skill:      newSkill,
		SourceFile: "",
		SoFile:     soFile,
		ConfigFile: configFile,
	}
	l.mu.Unlock()

	l.logger.Infof("✓ Loaded plugin: %s", skillID)
	return nil
}

// getSkillIDFromConfig 从配置文件读取 skill ID
func (l *Loader) getSkillIDFromConfig(configFile string) (string, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return "", err
	}

	var config struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return "", err
	}

	return config.ID, nil
}

// buildSkill 编译插件
func (l *Loader) buildSkill(goFiles []string, outputFile string) error {
	// 检查是否需要重新编译
	if l.needsRebuild(goFiles, outputFile) {
		args := append([]string{"build", "-buildmode=plugin", "-o", outputFile}, goFiles...)

		cmd := exec.Command("go", args...)
		// 设置工作目录为 .go 文件所在的目录
		cmd.Dir = filepath.Dir(goFiles[0])
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("compile error: %w, output: %s", err, output)
		}
	}

	return nil
}

// needsRebuild 检查是否需要重新编译
func (l *Loader) needsRebuild(goFiles []string, soFile string) bool {
	// 如果 .so 不存在，需要编译
	if _, err := os.Stat(soFile); os.IsNotExist(err) {
		return true
	}

	// 检查源文件是否比 .so 新
	soInfo, _ := os.Stat(soFile)
	soModTime := soInfo.ModTime()

	for _, goFile := range goFiles {
		if info, err := os.Stat(goFile); err == nil {
			if info.ModTime().After(soModTime) {
				return true
			}
		}
	}

	return false
}

// loadPlugin 加载 .so 插件
func (l *Loader) loadPlugin(soFile, skillID string) (Skill, error) {
	p, err := stdplugin.Open(soFile)
	if err != nil {
		return nil, err
	}

	// 查找 NewSkill 函数
	newSkillSym, err := p.Lookup("NewSkill")
	if err != nil {
		return nil, fmt.Errorf("NewSkill function not found: %w", err)
	}

	// 类型断言
	newSkillFunc, ok := newSkillSym.(func(*Dependencies) (Skill, error))
	if !ok {
		return nil, fmt.Errorf("invalid NewSkill signature")
	}

	// 调用创建技能
	skill, err := newSkillFunc(l.deps)
	if err != nil {
		return nil, fmt.Errorf("failed to create skill: %w", err)
	}

	return skill, nil
}

// ReloadSkill 重新加载指定技能
func (l *Loader) ReloadSkill(skillID string) error {
	l.mu.RLock()
	plugin, exists := l.plugins[skillID]
	l.mu.RUnlock()

	if !exists {
		return fmt.Errorf("skill not found: %s", skillID)
	}

	l.logger.Infof("Reloading skill: %s", skillID)

	// 重新编译
	if err := l.buildSkill([]string{plugin.SourceFile}, plugin.SoFile); err != nil {
		return fmt.Errorf("rebuild failed: %w", err)
	}

	// 加载新插件
	newSkill, err := l.loadPlugin(plugin.SoFile, skillID)
	if err != nil {
		return fmt.Errorf("reload failed, keeping old version: %w", err)
	}

	// 清理旧插件
	if plugin.Skill != nil {
		plugin.Skill.Cleanup()
	}

	// 更新
	l.mu.Lock()
	l.plugins[skillID] = Plugin{
		Skill:      newSkill,
		SourceFile: plugin.SourceFile,
		SoFile:     plugin.SoFile,
		ConfigFile: plugin.ConfigFile,
	}
	l.mu.Unlock()

	l.logger.Infof("✓ Skill %s reloaded", skillID)
	return nil
}

// GetSkill 获取已加载的技能
func (l *Loader) GetSkill(skillID string) (Skill, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	plugin, exists := l.plugins[skillID]
	if !exists {
		return nil, false
	}
	return plugin.Skill, true
}

// GetAllSkills 获取所有已加载的技能
func (l *Loader) GetAllSkills() []Skill {
	l.mu.RLock()
	defer l.mu.RUnlock()

	skills := make([]Skill, 0, len(l.plugins))
	for _, plugin := range l.plugins {
		skills = append(skills, plugin.Skill)
	}
	return skills
}

// StartWatcher 启动文件监听器
func (l *Loader) StartWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	// 监听 .so 文件目录
	if err := l.watchDir(watcher, l.binDir); err != nil {
		return err
	}

	go l.watchSoLoop(watcher)
	l.logger.Info("File watcher started for .so files in: ", l.binDir)

	return nil
}

// watchDir 递归监听目录
func (l *Loader) watchDir(watcher *fsnotify.Watcher, dir string) error {
	return watcher.Add(dir)
}

// watchSoLoop 监听 .so 文件循环
func (l *Loader) watchSoLoop(watcher *fsnotify.Watcher) {
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			// 只监听 .so 文件
			if filepath.Ext(event.Name) != ".so" {
				continue
			}

			// 从文件名提取 skill ID
			skillID := strings.TrimSuffix(filepath.Base(event.Name), ".so")

			// 防抖：500ms 后再处理
			time.AfterFunc(500*time.Millisecond, func() {
				l.handleSoFileChange(skillID, event.Name)
			})

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			l.logger.Errorf("Watcher error: %v", err)
		}
	}
}

// isSkillFile 判断是否是技能相关文件
func (l *Loader) isSkillFile(filename string) bool {
	ext := filepath.Ext(filename)
	return ext == ".go" || ext == ".json"
}

// getSkillDir 从文件路径提取技能目录
func (l *Loader) getSkillDir(filename string) string {
	// skills/financial/add_bill.go -> financial
	rel, err := filepath.Rel(l.skillsDir, filename)
	if err != nil {
		return ""
	}

	parts := filepath.SplitList(rel)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// handleSoFileChange 处理 .so 文件变更
func (l *Loader) handleSoFileChange(skillID, soFile string) {
	l.logger.Infof("Reloading skill: %s", skillID)

	// 加载新插件
	newSkill, err := l.loadPlugin(soFile, skillID)
	if err != nil {
		l.logger.Warnf("Failed to reload skill %s: %v", skillID, err)
		return
	}

	l.mu.Lock()
	// 清理旧插件（如果存在）
	if oldPlugin, exists := l.plugins[skillID]; exists && oldPlugin.Skill != nil {
		oldPlugin.Skill.Cleanup()
	}

	l.plugins[skillID] = Plugin{
		Skill:      newSkill,
		SourceFile: "",
		SoFile:     soFile,
		ConfigFile: "",
	}
	l.mu.Unlock()

	l.logger.Infof("✓ Skill %s reloaded", skillID)

	// 调用回调函数，通知主程序重新注册
	if l.OnReload != nil {
		l.OnReload(skillID, newSkill)
	}
}

// handleFileChange 处理文件变更
func (l *Loader) handleFileChange(skillDir string) {
	// 重新加载该目录下的技能
	skillPath := filepath.Join(l.skillsDir, skillDir)
	if err := l.loadSkillFromDir(skillPath); err != nil {
		l.logger.Warnf("Failed to reload skill from %s: %v", skillDir, err)
	}
}

// Cleanup 清理所有插件
func (l *Loader) Cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	for skillID, plugin := range l.plugins {
		if plugin.Skill != nil {
			plugin.Skill.Cleanup()
		}
		delete(l.plugins, skillID)
	}
}

// openPlugin 打开插件
func openPlugin(path string) (*stdplugin.Plugin, error) {
	return stdplugin.Open(path)
}
