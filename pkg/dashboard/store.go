package dashboard

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

var (
	ErrNotFound = errors.New("item not found")
	ErrConflict = errors.New("destination already exists")
	ErrUnsafe   = errors.New("this tool cannot be moved safely")
)

type ToolPreference struct {
	Alias    string       `json:"alias"`
	Domain   types.Domain `json:"domain,omitempty"`
	Notes    string       `json:"notes"`
	Tags     []string     `json:"tags"`
	Favorite bool         `json:"favorite"`
	Hidden   bool         `json:"hidden"`
	Updated  time.Time    `json:"updated"`
}

type Shortcut struct {
	ID      string    `json:"id"`
	Label   string    `json:"label"`
	Command string    `json:"command"`
	Updated time.Time `json:"updated"`
}

type Preset struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	ToolIDs []string  `json:"tool_ids"`
	Updated time.Time `json:"updated"`
}

type StandbyRecord struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	OriginalPath string    `json:"original_path"`
	StandbyPath  string    `json:"standby_path"`
	MovedAt      time.Time `json:"moved_at"`
}

type TrashRecord struct {
	ID         string    `json:"id"`
	ToolID     string    `json:"tool_id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Operation  string    `json:"operation"`
	SourcePath string    `json:"source_path"`
	VaultPath  string    `json:"vault_path"`
	RemovedAt  time.Time `json:"removed_at"`
}

type UserState struct {
	Version   int                       `json:"version"`
	Tools     map[string]ToolPreference `json:"tools"`
	Shortcuts map[string]Shortcut       `json:"shortcuts"`
	Presets   map[string]Preset         `json:"presets"`
	Standby   map[string]StandbyRecord  `json:"standby"`
	Trash     map[string]TrashRecord    `json:"trash"`
}

type Store struct {
	mu   sync.Mutex
	path string
	home string
}

func newStore(path, home string) *Store { return &Store{path: path, home: home} }

func defaultStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home), nil
}

// ReadSavedState provides agent-facing CLI consumers the same local metadata as the UI.
func ReadSavedState() (UserState, error) {
	store, err := defaultStore()
	if err != nil {
		return UserState{}, err
	}
	return store.View()
}

func emptyState() UserState {
	return UserState{Version: 1, Tools: map[string]ToolPreference{}, Shortcuts: map[string]Shortcut{}, Presets: map[string]Preset{}, Standby: map[string]StandbyRecord{}, Trash: map[string]TrashRecord{}}
}

func (s *Store) readLocked() (UserState, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return emptyState(), nil
	}
	if err != nil {
		return UserState{}, err
	}
	var state UserState
	if err := json.Unmarshal(data, &state); err != nil {
		return UserState{}, fmt.Errorf("invalid dashboard state: %w", err)
	}
	if state.Version != 1 {
		return UserState{}, errors.New("unsupported dashboard state version")
	}
	if state.Tools == nil {
		state.Tools = map[string]ToolPreference{}
	}
	if state.Shortcuts == nil {
		state.Shortcuts = map[string]Shortcut{}
	}
	if state.Presets == nil {
		state.Presets = map[string]Preset{}
	}
	if state.Standby == nil {
		state.Standby = map[string]StandbyRecord{}
	}
	if state.Trash == nil {
		state.Trash = map[string]TrashRecord{}
	}
	return state, nil
}

func (s *Store) saveLocked(state UserState) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".dashboard-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
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
	return os.Rename(file.Name(), s.path)
}

func (s *Store) View() (UserState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readLocked()
}

func (s *Store) SetTool(id string, preference ToolPreference) error {
	preference.Alias = strings.TrimSpace(preference.Alias)
	preference.Notes = strings.TrimSpace(preference.Notes)
	if len(preference.Alias) > 80 || len(preference.Notes) > 2000 {
		return errors.New("alias or notes too long")
	}
	if len(preference.Tags) > 12 {
		return errors.New("too many tags")
	}
	if preference.Domain != "" {
		allowed := map[types.Domain]bool{
			types.DomainGeneral: true, types.DomainFrontend: true, types.DomainBackend: true,
			types.DomainDevOps: true, types.DomainOrchestration: true, types.DomainTesting: true,
			types.DomainSecurity: true, types.DomainAI: true, types.DomainReverseEng: true,
			types.DomainContent: true,
		}
		if !allowed[preference.Domain] {
			return errors.New("invalid domain override")
		}
	}
	seenTags := map[string]bool{}
	for index, tag := range preference.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || len(tag) > 36 || strings.ContainsAny(tag, "/\\\\") || strings.IndexFunc(tag, func(r rune) bool { return r < 32 }) >= 0 || seenTags[strings.ToLower(tag)] {
			return errors.New("invalid or repeated tag")
		}
		seenTags[strings.ToLower(tag)] = true
		preference.Tags[index] = tag
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readLocked()
	if err != nil {
		return err
	}
	preference.Updated = time.Now().UTC()
	state.Tools[id] = preference
	return s.saveLocked(state)
}

func (s *Store) DeleteTool(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readLocked()
	if err != nil {
		return err
	}
	delete(state.Tools, id)
	return s.saveLocked(state)
}

func randomID() (string, error) {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func (s *Store) SaveShortcut(shortcut Shortcut) (Shortcut, error) {
	shortcut.Label = strings.TrimSpace(shortcut.Label)
	shortcut.Command = strings.TrimSpace(shortcut.Command)
	if shortcut.Label == "" || shortcut.Command == "" || len(shortcut.Label) > 80 || len(shortcut.Command) > 500 {
		return Shortcut{}, errors.New("shortcut label or command is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readLocked()
	if err != nil {
		return Shortcut{}, err
	}
	if shortcut.ID == "" {
		if len(state.Shortcuts) >= 100 {
			return Shortcut{}, errors.New("shortcut limit reached")
		}
		shortcut.ID, err = randomID()
		if err != nil {
			return Shortcut{}, err
		}
	} else if _, exists := state.Shortcuts[shortcut.ID]; !exists {
		return Shortcut{}, ErrNotFound
	}
	shortcut.Updated = time.Now().UTC()
	state.Shortcuts[shortcut.ID] = shortcut
	return shortcut, s.saveLocked(state)
}

func (s *Store) DeleteShortcut(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readLocked()
	if err != nil {
		return err
	}
	if _, exists := state.Shortcuts[id]; !exists {
		return ErrNotFound
	}
	delete(state.Shortcuts, id)
	return s.saveLocked(state)
}

func (s *Store) SavePreset(preset Preset) (Preset, error) {
	preset.Name = strings.TrimSpace(preset.Name)
	if preset.Name == "" || len(preset.Name) > 80 || len(preset.ToolIDs) > 200 {
		return Preset{}, errors.New("invalid preset")
	}
	seen := map[string]bool{}
	for _, id := range preset.ToolIDs {
		if id == "" || len(id) > 500 || seen[id] {
			return Preset{}, errors.New("invalid or repeated tool ID")
		}
		seen[id] = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readLocked()
	if err != nil {
		return Preset{}, err
	}
	if preset.ID == "" {
		if len(state.Presets) >= 50 {
			return Preset{}, errors.New("preset limit reached")
		}
		preset.ID, err = randomID()
		if err != nil {
			return Preset{}, err
		}
	} else if _, ok := state.Presets[preset.ID]; !ok {
		return Preset{}, ErrNotFound
	}
	preset.Updated = time.Now().UTC()
	state.Presets[preset.ID] = preset
	return preset, s.saveLocked(state)
}

func (s *Store) DeletePreset(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readLocked()
	if err != nil {
		return err
	}
	if _, exists := state.Presets[id]; !exists {
		return ErrNotFound
	}
	delete(state.Presets, id)
	return s.saveLocked(state)
}

// CanStandby restricts physical changes to symlink aliases in the Gemini active set.
func (s *Store) CanStandby(item types.Entity) bool {
	root := filepath.Join(s.home, ".gemini", "config", "skills")
	return item.Type == types.TypeSkill && item.IsSymlink && filepath.Dir(filepath.Clean(item.Path)) == root && filepath.Base(item.Path) == item.Name
}

func (s *Store) MoveToStandby(item types.Entity) error {
	if !s.CanStandby(item) {
		return ErrUnsafe
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readLocked()
	if err != nil {
		return err
	}
	if _, exists := state.Standby[item.ID]; exists {
		return ErrConflict
	}
	info, err := os.Lstat(item.Path)
	if err != nil {
		return ErrNotFound
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return ErrUnsafe
	}
	vault := filepath.Join(s.home, ".ash", "vault", "standby")
	if err := os.MkdirAll(vault, 0700); err != nil {
		return err
	}
	dest := filepath.Join(vault, item.Name)
	if _, err := os.Lstat(dest); err == nil {
		return ErrConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(item.Path, dest); err != nil {
		return err
	}
	state.Standby[item.ID] = StandbyRecord{ID: item.ID, Name: item.Name, OriginalPath: item.Path, StandbyPath: dest, MovedAt: time.Now().UTC()}
	if err := s.saveLocked(state); err != nil {
		if rollback := os.Rename(dest, item.Path); rollback != nil {
			return fmt.Errorf("state save failed and rollback failed: %w", rollback)
		}
		return err
	}
	return nil
}

func (s *Store) RestoreFromStandby(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readLocked()
	if err != nil {
		return err
	}
	record, exists := state.Standby[id]
	if !exists {
		return ErrNotFound
	}
	root := filepath.Join(s.home, ".gemini", "config", "skills")
	vault := filepath.Join(s.home, ".ash", "vault", "standby")
	if filepath.Dir(filepath.Clean(record.OriginalPath)) != root || filepath.Dir(filepath.Clean(record.StandbyPath)) != vault || record.Name != filepath.Base(record.OriginalPath) || record.Name != filepath.Base(record.StandbyPath) {
		return ErrUnsafe
	}
	info, err := os.Lstat(record.StandbyPath)
	if err != nil {
		return ErrNotFound
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return ErrUnsafe
	}
	if _, err := os.Lstat(record.OriginalPath); err == nil {
		return ErrConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(record.StandbyPath, record.OriginalPath); err != nil {
		return err
	}
	delete(state.Standby, id)
	if err := s.saveLocked(state); err != nil {
		if rollback := os.Rename(record.OriginalPath, record.StandbyPath); rollback != nil {
			return fmt.Errorf("state save failed and rollback failed: %w", rollback)
		}
		return err
	}
	return nil
}
