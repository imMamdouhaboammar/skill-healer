package instrument

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var safeExecutableName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.+-]{0,79}$`)

func CLIShimsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ash", "shims")
}

// FindOriginalCLI resolves executable paths without accidentally selecting an ash shim.
func FindOriginalCLI(name, shims string) (string, error) {
	if !safeExecutableName.MatchString(name) || name == "ash" {
		return "", fmt.Errorf("unsafe CLI name: %q", name)
	}
	search := append(filepath.SplitList(os.Getenv("PATH")), "/opt/homebrew/bin", "/usr/local/bin")
	if home, err := os.UserHomeDir(); err == nil {
		search = append(search, filepath.Join(home, ".local", "bin"))
	}
	for _, dir := range search {
		if dir == "" || filepath.Clean(dir) == filepath.Clean(shims) {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
			resolved, err := filepath.Abs(path)
			if err == nil {
				return resolved, nil
			}
		}
	}
	return "", exec.ErrNotFound
}

// InstallCLIShim creates a reversible wrapper which records before executing
// the original binary with the exact original arguments and exit status.
func InstallCLIShim(shims, binary, name string) (string, error) {
	if !filepath.IsAbs(binary) {
		return "", fmt.Errorf("ash binary path must be absolute")
	}
	if shims == "" {
		shims = CLIShimsDir()
	}
	original, err := FindOriginalCLI(name, shims)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(shims, 0700); err != nil {
		return "", err
	}
	wrapper := filepath.Join(shims, name)
	text := "#!/bin/sh\n# AGentic Skills Healer managed shim: " + name + "\n" +
		shellQuote(binary) + " hook cli " + shellQuote(name) + " >/dev/null 2>&1 || :\n" +
		"exec " + shellQuote(original) + " \"$@\"\n"
	if data, err := os.ReadFile(wrapper); err == nil {
		if string(data) == text {
			return wrapper, nil
		}
		if !strings.HasPrefix(string(data), "#!/bin/sh\n# AGentic Skills Healer managed shim: "+name+"\n") {
			return "", fmt.Errorf("existing unmanaged file at %s", wrapper)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	tmp, err := os.CreateTemp(shims, ".ash-cli-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.WriteString(text); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err = tmp.Chmod(0700); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err = tmp.Close(); err != nil {
		return "", err
	}
	if err = os.Rename(tmp.Name(), wrapper); err != nil {
		return "", err
	}
	return wrapper, nil
}

const zshMarker = "# AGentic Skills Healer CLI tracking shims"

// EnableZshShims adds a single PATH entry for future interactive zsh sessions.
func EnableZshShims(rcPath, shims string) (bool, error) {
	if rcPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false, err
		}
		rcPath = filepath.Join(home, ".zshrc")
	}
	if shims == "" {
		shims = CLIShimsDir()
	}
	if !filepath.IsAbs(shims) {
		return false, fmt.Errorf("shim path must be absolute")
	}
	existing, err := os.ReadFile(rcPath)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if strings.Contains(string(existing), zshMarker) {
		return false, nil
	}
	if exists {
		fi, err := os.Lstat(rcPath)
		if err != nil {
			return false, err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return false, fmt.Errorf("shell config is a symlink; configure PATH manually")
		}
		backup := rcPath + ".ash-backup"
		if _, err = os.Stat(backup); err == nil {
			return false, fmt.Errorf("backup already exists at %s", backup)
		}
		if err = os.WriteFile(backup, existing, 0600); err != nil {
			return false, fmt.Errorf("backing up shell config: %w", err)
		}
	}
	content := append(append([]byte{}, existing...), []byte("\n"+zshMarker+"\nexport PATH="+shellQuote(shims)+":\"$PATH\"\n")...)
	tmp, err := os.CreateTemp(filepath.Dir(rcPath), ".ash-zsh-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(content); err != nil {
		_ = tmp.Close()
		return false, err
	}
	perm := os.FileMode(0600)
	if exists {
		if fi, err := os.Stat(rcPath); err == nil {
			perm = fi.Mode().Perm()
		}
	}
	if err = tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return false, err
	}
	if err = tmp.Close(); err != nil {
		return false, err
	}
	if err = os.Rename(tmp.Name(), rcPath); err != nil {
		return false, err
	}
	return true, nil
}
