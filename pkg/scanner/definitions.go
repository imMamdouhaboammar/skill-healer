package scanner

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

func readLimited(path string, limit int64) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit))
	if err != nil {
		return ""
	}
	return string(data)
}

func metadataValue(content, key string) string {
	content = strings.TrimPrefix(content, "\ufeff")
	if strings.HasPrefix(content, "---") {
		lines := strings.Split(content, "\n")
		for _, line := range lines[1:] {
			if strings.TrimSpace(line) == "---" {
				break
			}
			pair := strings.SplitN(strings.TrimSpace(line), ":", 2)
			if len(pair) == 2 && strings.EqualFold(strings.TrimSpace(pair[0]), key) {
				return strings.Trim(strings.TrimSpace(pair[1]), "'\"")
			}
		}
	}
	// Small subset of single-line TOML fields for Codex agents
	for _, line := range strings.Split(content, "\n") {
		pair := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(pair) == 2 && strings.EqualFold(strings.TrimSpace(pair[0]), key) {
			return strings.Trim(strings.TrimSpace(pair[1]), "'\"")
		}
	}
	return ""
}

func readDescription(path string) string {
	content := readLimited(path, 12<<10)
	description := metadataValue(content, "description")
	if description == "" {
		reader := bufio.NewScanner(strings.NewReader(content))
		for reader.Scan() {
			line := strings.TrimSpace(reader.Text())
			if strings.HasPrefix(line, "# ") {
				description = strings.TrimPrefix(line, "# ")
				break
			}
		}
	}
	if len(description) > 320 {
		description = description[:320]
	}
	return description
}

func (c *collector) scanSkillRoot(root, scope, source string) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return
	}
	c.recordRoot(root)
	count := 0
	var walk func(path string, depth int)
	walk = func(path string, depth int) {
		if depth > 4 || count > 12000 {
			return
		}
		count++
		if path != root {
			manifest := filepath.Join(path, "SKILL.md")
			if fi, err := os.Stat(manifest); err == nil && fi.Mode().IsRegular() {
				c.add(types.TypeSkill, filepath.Base(path), path, scope, source, readDescription(manifest), "installed")
				return
			}
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			c.warn("Cannot read skills directory: %s", path)
			return
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			child := filepath.Join(path, entry.Name())
			info, err := os.Stat(child)
			if err == nil && info.IsDir() {
				walk(child, depth+1)
			}
		}
	}
	walk(root, 0)
	if count > 12000 {
		c.warn("Skill root reached traversal cap: %s", root)
	}
}

func (c *collector) scanAgentRoot(root, scope, source string) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return
	}
	c.recordRoot(root)
	var walk func(path string, depth int)
	walk = func(path string, depth int) {
		if depth > 3 {
			return
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			c.warn("Cannot read agents directory: %s", path)
			return
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			full := filepath.Join(path, entry.Name())
			if entry.IsDir() {
				walk(full, depth+1)
				continue
			}
			extension := strings.ToLower(filepath.Ext(entry.Name()))
			if extension != ".md" && extension != ".toml" {
				continue
			}
			content := readLimited(full, 12<<10)
			name := metadataValue(content, "name")
			if name == "" {
				name = strings.TrimSuffix(entry.Name(), extension)
			}
			desc := metadataValue(content, "description")
			c.add(types.TypeAgent, name, full, scope, source, desc, "installed")
		}
	}
	walk(root, 0)
}
