package scanner

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

var systemCLIs = map[string]bool{
	"claude": true, "codex": true, "gemini": true, "get-fable": true, "fable-dsh": true,
	"agent-kernel": true, "no-mistakes": true, "gbrain": true, "xerj": true, "riqor": true,
	"fastmcp": true, "rea": true, "rea-agents": true, "agentapi": true, "agent-browser": true,
	"bun": true, "cargo": true, "rustc": true, "go": true, "python3": true,
	"docker": true, "kubectl": true, "gh": true, "git": true, "lazygit": true,
	"gitleaks": true, "playwright": true, "tiny-agents": true,
}

func scanExecutableName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.HasPrefix(name, ".")
}

func (c *collector) scanBinDir(dir, scope, source string, restrictSystem bool) {
	if filepath.Base(dir) == "shims" && filepath.Base(filepath.Dir(dir)) == ".ash" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	c.recordRoot(dir)
	count := 0
	for _, entry := range entries {
		name := entry.Name()
		if !scanExecutableName(name) {
			continue
		}
		if restrictSystem && !systemCLIs[name] && !strings.HasPrefix(name, "agent-kernel-") && !strings.HasPrefix(name, "hostinger-") {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			continue
		}
		c.add(types.TypeCLI, name, path, scope, source, "", "executable")
		count++
		if count >= 6000 {
			c.warn("CLI bin scan capped at 6000: %s", dir)
			break
		}
	}
}

func (c *collector) scanCLIs(home string) {
	dirs := append([]string{}, filepath.SplitList(os.Getenv("PATH"))...)
	dirs = append(dirs, filepath.Join(home, ".local", "bin"), filepath.Join(home, ".bun", "bin"),
		filepath.Join(home, ".cargo", "bin"), filepath.Join(home, ".npm-global", "bin"),
		filepath.Join(home, "bin"), "/opt/homebrew/bin", "/usr/local/bin")
	visited := map[string]bool{}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		absolute, err := filepath.Abs(dir)
		if err != nil || visited[absolute] {
			continue
		}
		visited[absolute] = true
		system := strings.HasPrefix(absolute, "/usr/bin") || absolute == "/bin" ||
			strings.HasPrefix(absolute, "/usr/sbin") || absolute == "/sbin"
		c.scanBinDir(absolute, "global", "path", system)
	}
}

func (c *collector) scanProjectCLIs(project string) {
	for _, sub := range []string{"node_modules/.bin", ".venv/bin", "bin"} {
		dir := filepath.Join(project, sub)
		c.scanBinDir(dir, "project", "project-bin", false)
	}
}
