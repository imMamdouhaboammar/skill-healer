package dashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

type CleanupPlan struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	SourcePath  string `json:"source_path"`
	Allowed     bool   `json:"allowed"`
	Operation   string `json:"operation"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Warning     string `json:"warning,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type cleanupManager struct{ store *Store }

func newCleanupManager(store *Store) *cleanupManager { return &cleanupManager{store: store} }

func (c *cleanupManager) vaultRoot() string {
	return filepath.Join(c.store.home, ".ash", "vault", "trash")
}

func (c *cleanupManager) allowedProjectSource(item types.Entity) (string, string) {
	if item.Scope != "project" {
		return "", ""
	}
	path := filepath.Clean(item.Path)
	within := false
	for _, base := range []string{"Documents", "Projects", "Developer", "Code"} {
		root := filepath.Join(c.store.home, base)
		rel, err := filepath.Rel(root, path)
		if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			within = true
			break
		}
	}
	if !within {
		return "", ""
	}
	if item.Type == types.TypeMCP && filepath.Base(path) == ".mcp.json" {
		return "remove-mcp-entry", path
	}
	if item.Type != types.TypeSkill && item.Type != types.TypeAgent {
		return "", ""
	}
	parent := filepath.Dir(path)
	role := filepath.Base(parent)
	host := filepath.Base(filepath.Dir(parent))
	validHost := host == ".claude" || host == ".agents" || host == ".codex" || host == ".gemini" || host == ".cursor"
	if !validHost {
		return "", ""
	}
	if item.Type == types.TypeSkill && role == "skills" && filepath.Base(path) == item.Name {
		return "quarantine-path", parent
	}
	if item.Type == types.TypeAgent && role == "agents" &&
		(filepath.Base(path) == item.Name+".md" || filepath.Base(path) == item.Name+".toml") {
		return "quarantine-path", parent
	}
	return "", ""
}

func (c *cleanupManager) allowedSource(item types.Entity) (string, string) {
	if operation, root := c.allowedProjectSource(item); operation != "" {
		return operation, root
	}
	home := c.store.home
	paths := map[types.EntityType][]string{
		types.TypeSkill: {
			filepath.Join(home, ".agents", "skills"),
			filepath.Join(home, ".codex", "skills"),
			filepath.Join(home, ".gemini", "config", "skills"),
			filepath.Join(home, ".claude", "skills"),
			filepath.Join(home, ".gemini", "skills"),
			filepath.Join(home, ".cursor", "skills"),
		},
		types.TypeMCP: {filepath.Join(home, ".gemini", "antigravity", "mcp")},
		types.TypeCLI: {
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, "bin"),
			filepath.Join(home, ".bun", "bin"),
			filepath.Join(home, ".cargo", "bin"),
		},
		types.TypeAgent: {
			filepath.Join(home, ".claude", "agents"),
			filepath.Join(home, ".codex", "agents"),
			filepath.Join(home, ".agents", "agents"),
			filepath.Join(home, ".gemini", "agents"),
			filepath.Join(home, ".cursor", "agents"),
		},
	}
	if item.Name == "" || item.Name == "." || item.Name == ".." || item.Path == "" {
		return "", ""
	}
	if item.Type == types.TypeCLI && item.Name == "ash" {
		return "", ""
	}
	if item.Type == types.TypeMCP && item.Scope == "project" && filepath.Clean(item.Path) == filepath.Join(home, ".claude.json") {
		return "", ""
	}
	path := filepath.Clean(item.Path)
	for _, root := range paths[item.Type] {
		if filepath.Dir(path) == root && filepath.Base(path) == item.Name {
			return "quarantine-path", root
		}
		if item.Type == types.TypeAgent && filepath.Dir(path) == root && filepath.Base(path) == item.Name+".md" {
			return "quarantine-path", root
		}
		if item.Type == types.TypeAgent && filepath.Dir(path) == root && filepath.Base(path) == item.Name+".toml" {
			return "quarantine-path", root
		}
	}
	if item.Type == types.TypeMCP {
		for _, path := range []string{
			filepath.Join(home, ".gemini", "antigravity", "mcp_config.json"),
			filepath.Join(home, ".claude.json"),
			filepath.Join(home, ".claude", "settings.json"),
			filepath.Join(home, ".gemini", "settings.json"),
			filepath.Join(home, ".cursor", "mcp.json"),
		} {
			if filepath.Clean(item.Path) == path {
				return "remove-mcp-entry", path
			}
		}
	}
	return "", ""
}

func fingerprint(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:])
}

func fileFingerprint(path string, info os.FileInfo) (string, error) {
	target := ""
	if info.Mode()&os.ModeSymlink != 0 {
		var err error
		target, err = os.Readlink(path)
		if err != nil {
			return "", err
		}
	}
	return fingerprint(path, info.Mode().String(), fmt.Sprint(info.Size()), fmt.Sprint(info.ModTime().UnixNano()), target), nil
}

func (c *cleanupManager) Plan(item types.Entity) (CleanupPlan, error) {
	plan := CleanupPlan{ID: item.ID, Name: item.Name, Type: string(item.Type), SourcePath: item.Path}
	operation, root := c.allowedSource(item)
	if operation == "" {
		plan.Reason = "This source has no verified local removal adapter. System or package-managed binaries and virtual agents require their own uninstall method"
		return plan, nil
	}
	if operation == "quarantine-path" {
		info, err := os.Lstat(item.Path)
		if err != nil {
			plan.Reason = "The source is missing or cannot be accessed"
			return plan, nil
		}
		fp, err := fileFingerprint(item.Path, info)
		if err != nil {
			return CleanupPlan{}, err
		}
		plan.Fingerprint = fp
		plan.Allowed = true
		plan.Operation = operation
		plan.Warning = "Moves this installed entry out of its active directory into the private trash vault. It can be restored before permanent deletion"
		if item.Attribution == "ambiguous_name" {
			plan.Warning += ". Usage records are shared across same-name installations; verify the exact source"
		}
		if root == filepath.Join(c.store.home, ".agents", "skills") {
			plan.Warning += ". This is a canonical skills location, so other agents may reference the removed content"
		}
		return plan, nil
	}
	data, _, _, err := readMCP(item.Path, item.Name)
	if err != nil {
		plan.Reason = "This MCP entry is missing, malformed or inaccessible"
		return plan, nil
	}
	plan.Allowed = true
	plan.Operation = operation
	plan.Fingerprint = fingerprint(item.Path, item.Name, string(data))
	plan.Warning = "Removes only this MCP server registration from the JSON config. A private recovery copy is retained until permanent deletion"
	if item.Attribution == "ambiguous_name" {
		plan.Warning += ". Multiple registrations share this name; verify the exact source"
	}
	return plan, nil
}

func readMCP(path, name string) ([]byte, map[string]json.RawMessage, json.RawMessage, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, nil, ErrUnsafe
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, err
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, nil, nil, err
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(document["mcpServers"], &entries); err != nil {
		return nil, nil, nil, err
	}
	entry, ok := entries[name]
	if !ok {
		return nil, nil, nil, ErrNotFound
	}
	return data, document, entry, nil
}

func writeMCP(path string, doc map[string]json.RawMessage, key string, entry *json.RawMessage) error {
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(doc["mcpServers"], &servers); err != nil {
		return err
	}
	if entry == nil {
		delete(servers, key)
	} else {
		servers[key] = *entry
	}
	updated, err := json.Marshal(servers)
	if err != nil {
		return err
	}
	doc["mcpServers"] = updated
	output, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	output = append(output, '\n')
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return writeAtomic(path, output, info.Mode().Perm())
}

func writeAtomic(path string, data []byte, perm os.FileMode) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".ash-cleanup-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(perm); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func (c *cleanupManager) allocateVault() (string, string, error) {
	if err := os.MkdirAll(c.vaultRoot(), 0700); err != nil {
		return "", "", err
	}
	id, err := randomID()
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(c.vaultRoot(), id)
	if err := os.Mkdir(path, 0700); err != nil {
		return "", "", err
	}
	return id, path, nil
}

func (c *cleanupManager) Remove(item types.Entity, expectedFingerprint, confirmation string) (TrashRecord, error) {
	if confirmation != item.Name || item.Name == "" || expectedFingerprint == "" {
		return TrashRecord{}, ErrUnsafe
	}
	c.store.mu.Lock()
	defer c.store.mu.Unlock()
	plan, err := c.Plan(item)
	if err != nil {
		return TrashRecord{}, err
	}
	if !plan.Allowed {
		return TrashRecord{}, ErrUnsafe
	}
	if expectedFingerprint != plan.Fingerprint {
		return TrashRecord{}, ErrConflict
	}
	state, err := c.store.readLocked()
	if err != nil {
		return TrashRecord{}, err
	}
	id, vault, err := c.allocateVault()
	if err != nil {
		return TrashRecord{}, err
	}
	record := TrashRecord{ID: id, ToolID: item.ID, Name: item.Name, Type: string(item.Type), Operation: plan.Operation, SourcePath: item.Path, VaultPath: vault, RemovedAt: time.Now().UTC()}
	if plan.Operation == "quarantine-path" {
		return c.removePath(record, state, expectedFingerprint)
	}
	return c.removeMCP(record, state, expectedFingerprint)
}

func (c *cleanupManager) removePath(record TrashRecord, state UserState, expectedFingerprint string) (TrashRecord, error) {
	info, err := os.Lstat(record.SourcePath)
	if err != nil {
		return TrashRecord{}, err
	}
	current, err := fileFingerprint(record.SourcePath, info)
	if err != nil {
		return TrashRecord{}, err
	}
	if current != expectedFingerprint {
		return TrashRecord{}, ErrConflict
	}
	payload := filepath.Join(record.VaultPath, "payload")
	if err := os.Rename(record.SourcePath, payload); err != nil {
		return TrashRecord{}, err
	}
	state.Trash[record.ID] = record
	if err := c.store.saveLocked(state); err != nil {
		if rollback := os.Rename(payload, record.SourcePath); rollback != nil {
			return TrashRecord{}, fmt.Errorf("metadata save and rollback failed: %w", rollback)
		}
		return TrashRecord{}, err
	}
	return record, nil
}

func (c *cleanupManager) removeMCP(record TrashRecord, state UserState, expectedFingerprint string) (TrashRecord, error) {
	before, doc, entry, err := readMCP(record.SourcePath, record.Name)
	if err != nil {
		return TrashRecord{}, err
	}
	if fingerprint(record.SourcePath, record.Name, string(before)) != expectedFingerprint {
		return TrashRecord{}, ErrConflict
	}
	info, err := os.Stat(record.SourcePath)
	if err != nil {
		return TrashRecord{}, err
	}
	if err := os.WriteFile(filepath.Join(record.VaultPath, "entry.json"), entry, 0600); err != nil {
		return TrashRecord{}, err
	}
	if err := os.WriteFile(filepath.Join(record.VaultPath, "original.json"), before, 0600); err != nil {
		return TrashRecord{}, err
	}
	if err := writeMCP(record.SourcePath, doc, record.Name, nil); err != nil {
		return TrashRecord{}, err
	}
	state.Trash[record.ID] = record
	if err := c.store.saveLocked(state); err != nil {
		if rollback := writeAtomic(record.SourcePath, before, info.Mode().Perm()); rollback != nil {
			return TrashRecord{}, fmt.Errorf("metadata save and rollback failed: %w", rollback)
		}
		return TrashRecord{}, err
	}
	return record, nil
}

func (c *cleanupManager) recordLocked(id string) (UserState, TrashRecord, error) {
	state, err := c.store.readLocked()
	if err != nil {
		return UserState{}, TrashRecord{}, err
	}
	record, exists := state.Trash[id]
	if !exists {
		return UserState{}, TrashRecord{}, ErrNotFound
	}
	if len(id) != 24 || filepath.Dir(filepath.Clean(record.VaultPath)) != c.vaultRoot() || filepath.Base(record.VaultPath) != id {
		return UserState{}, TrashRecord{}, ErrUnsafe
	}
	return state, record, nil
}

func (c *cleanupManager) Restore(id string) error {
	c.store.mu.Lock()
	defer c.store.mu.Unlock()
	state, record, err := c.recordLocked(id)
	if err != nil {
		return err
	}
	if record.Operation == "quarantine-path" {
		err = c.restorePath(record, &state)
	} else if record.Operation == "remove-mcp-entry" {
		err = c.restoreMCP(record, &state)
	} else {
		return ErrUnsafe
	}
	if err != nil {
		return err
	}
	// Only a verified, private per-record vault directory is ever cleaned up.
	return os.RemoveAll(record.VaultPath)
}

func (c *cleanupManager) restorePath(record TrashRecord, state *UserState) error {
	item := types.Entity{Type: types.EntityType(record.Type), Name: record.Name, Path: record.SourcePath}
	if op, _ := c.allowedSource(item); op != "quarantine-path" {
		return ErrUnsafe
	}
	if _, err := os.Lstat(record.SourcePath); err == nil {
		return ErrConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	payload := filepath.Join(record.VaultPath, "payload")
	if _, err := os.Lstat(payload); err != nil {
		return ErrNotFound
	}
	if err := os.Rename(payload, record.SourcePath); err != nil {
		return err
	}
	delete(state.Trash, record.ID)
	if err := c.store.saveLocked(*state); err != nil {
		if rollback := os.Rename(record.SourcePath, payload); rollback != nil {
			return fmt.Errorf("metadata save and rollback failed: %w", rollback)
		}
		return err
	}
	return nil
}

func (c *cleanupManager) restoreMCP(record TrashRecord, state *UserState) error {
	item := types.Entity{Type: types.TypeMCP, Name: record.Name, Path: record.SourcePath}
	if op, _ := c.allowedSource(item); op != "remove-mcp-entry" {
		return ErrUnsafe
	}
	entry, err := os.ReadFile(filepath.Join(record.VaultPath, "entry.json"))
	if err != nil {
		return ErrNotFound
	}
	var value json.RawMessage = entry
	before, err := os.ReadFile(record.SourcePath)
	if err != nil {
		return err
	}
	info, err := os.Stat(record.SourcePath)
	if err != nil {
		return err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(before, &doc); err != nil {
		return err
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(doc["mcpServers"], &servers); err != nil {
		return err
	}
	if _, exists := servers[record.Name]; exists {
		return ErrConflict
	}
	if err := writeMCP(record.SourcePath, doc, record.Name, &value); err != nil {
		return err
	}
	delete(state.Trash, record.ID)
	if err := c.store.saveLocked(*state); err != nil {
		if rollback := writeAtomic(record.SourcePath, before, info.Mode().Perm()); rollback != nil {
			return fmt.Errorf("metadata save and rollback failed: %w", rollback)
		}
		return err
	}
	return nil
}

func (c *cleanupManager) Purge(id, confirmation string) error {
	c.store.mu.Lock()
	defer c.store.mu.Unlock()
	state, record, err := c.recordLocked(id)
	if err != nil {
		return err
	}
	if confirmation != "DELETE "+record.Name {
		return ErrUnsafe
	}
	if err := os.RemoveAll(record.VaultPath); err != nil {
		return err
	}
	delete(state.Trash, id)
	return c.store.saveLocked(state)
}
