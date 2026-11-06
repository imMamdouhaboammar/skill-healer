package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ignoredProjectDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true,
	".next": true, ".nuxt": true, ".venv": true, "venv": true, "target": true,
	".cache": true, "__pycache__": true, "coverage": true, ".turbo": true,
	".gradle": true, "Pods": true, ".terraform": true, ".ash": true,
}

func configuredProjectRoots(home string) []string {
	candidates := []string{}
	if raw := strings.TrimSpace(os.Getenv("ASH_PROJECT_ROOTS")); raw != "" {
		candidates = append(candidates, filepath.SplitList(raw)...)
	} else {
		for _, name := range []string{"Documents", "Projects", "Developer", "Code"} {
			candidates = append(candidates, filepath.Join(home, name))
		}
		file := filepath.Join(home, ".ash", "projects.json")
		if data, err := os.ReadFile(file); err == nil {
			var config struct {
				Roots []string `json:"roots"`
			}
			if json.Unmarshal(data, &config) == nil {
				candidates = append(candidates, config.Roots...)
			}
		}
	}
	seen := map[string]bool{}
	roots := []string{}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if candidate == "~" {
			candidate = home
		}
		if strings.HasPrefix(candidate, "~/") {
			candidate = filepath.Join(home, strings.TrimPrefix(candidate, "~/"))
		}
		absolute, err := filepath.Abs(candidate)
		if err != nil || seen[absolute] {
			continue
		}
		seen[absolute] = true
		roots = append(roots, absolute)
	}
	return roots
}

func (c *collector) scanProjects(home string) {
	for _, root := range configuredProjectRoots(home) {
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			if len(c.pool.Coverage.Skipped) < 80 {
				c.pool.Coverage.Skipped = append(c.pool.Coverage.Skipped, root)
			}
			continue
		}
		c.recordRoot(root)
		visited := 0
		deadline := time.Now().Add(5 * time.Second)
		var walk func(path string, depth int)
		walk = func(path string, depth int) {
			if visited >= 6000 || time.Now().After(deadline) || depth > 5 {
				return
			}
			visited++
			c.scanProject(path)
			entries, err := os.ReadDir(path)
			if err != nil {
				c.warn("Cannot read project directory: %s", path)
				return
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				name := entry.Name()
				if strings.HasPrefix(name, ".") || ignoredProjectDirs[name] {
					continue
				}
				walk(filepath.Join(path, name), depth+1)
			}
		}
		walk(root, 0)
		if visited >= 6000 || time.Now().After(deadline) {
			c.warn("Project discovery budget reached under %s; results partial", root)
		}
	}
}

func (c *collector) scanProject(path string) {
	hasSource := false
	for _, marker := range []string{"package.json", "go.mod", "Cargo.toml", "pyproject.toml", ".git"} {
		if _, err := os.Stat(filepath.Join(path, marker)); err == nil {
			c.scanProjectCLIs(path)
			break
		}
	}
	for _, spec := range []struct{ relative, source string }{
		{".claude/skills", "claude"}, {".agents/skills", "agents"}, {".codex/skills", "codex"},
		{".gemini/skills", "gemini"}, {".cursor/skills", "cursor"},
	} {
		root := filepath.Join(path, spec.relative)
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			hasSource = true
			c.scanSkillRoot(root, "project", spec.source)
		}
	}
	for _, spec := range []struct{ relative, source string }{
		{".claude/agents", "claude"}, {".agents/agents", "agents"}, {".codex/agents", "codex"},
		{".gemini/agents", "gemini"}, {".cursor/agents", "cursor"},
	} {
		root := filepath.Join(path, spec.relative)
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			hasSource = true
			c.scanAgentRoot(root, "project", spec.source)
		}
	}
	for _, spec := range []struct{ relative, source string }{
		{".mcp.json", "claude"}, {".claude/settings.json", "claude"},
		{".gemini/settings.json", "gemini"}, {".codex/config.toml", "codex"},
		{".cursor/mcp.json", "cursor"}, {".vscode/mcp.json", "vscode"},
	} {
		config := filepath.Join(path, spec.relative)
		if info, err := os.Stat(config); err == nil && info.Mode().IsRegular() {
			hasSource = true
			c.scanMCPConfig(config, "project", spec.source)
		}
	}
	if hasSource {
		c.pool.Coverage.ProjectDirectories++
	}
}
