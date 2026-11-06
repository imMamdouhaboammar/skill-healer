package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

func DefaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "ash-telemetry.json")
	}
	return filepath.Join(home, ".ash", "db", "telemetry.json")
}

// Load reads a fully committed snapshot. Writers commit with an atomic rename.
func Load(path string) (*types.TelemetryDB, error) {
	if path == "" {
		path = DefaultDBPath()
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &types.TelemetryDB{
			Version:     "1.0.0",
			LastUpdated: float64(time.Now().Unix()),
			Entities:    make(map[string]types.TelemetryRecord),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading telemetry db: %w", err)
	}
	var db types.TelemetryDB
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, fmt.Errorf("parsing telemetry db: %w", err)
	}
	if db.Entities == nil {
		db.Entities = make(map[string]types.TelemetryRecord)
	}
	return &db, nil
}

// lockDatabase uses a kernel advisory lock shared by independent processes.
func lockDatabase(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening db lock: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("locking db: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		_ = lock.Close()
	}, nil
}

// saveUnlocked must only be called while holding the database lock.
func saveUnlocked(path string, db *types.TelemetryDB) error {
	if db == nil {
		return fmt.Errorf("nil telemetry db")
	}
	db.LastUpdated = float64(time.Now().Unix())
	db.TotalEntities = len(db.Entities)
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling telemetry db: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".telemetry-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary db: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing telemetry db: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing telemetry db: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("closing telemetry db: %w", err)
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("committing telemetry db: %w", err)
	}
	return nil
}

func Save(path string, db *types.TelemetryDB) error {
	if path == "" {
		path = DefaultDBPath()
	}
	unlock, err := lockDatabase(path)
	if err != nil {
		return err
	}
	defer unlock()
	return saveUnlocked(path, db)
}

// RecordInvocation records caller-asserted usage and retains backward compatibility.
func RecordInvocation(path string, entityType types.EntityType, name string) (*types.TelemetryRecord, error) {
	return record(path, entityType, name, "")
}

// RecordObservedInvocation increments both the historical and verified counters.
// Its source must be a concrete host hook or command shim.
func RecordObservedInvocation(path string, entityType types.EntityType, name, source string) (*types.TelemetryRecord, error) {
	if strings.TrimSpace(source) == "" {
		return nil, fmt.Errorf("observed invocation requires source")
	}
	return record(path, entityType, name, source)
}

func record(path string, entityType types.EntityType, name, source string) (*types.TelemetryRecord, error) {
	if path == "" {
		path = DefaultDBPath()
	}
	if entityType != types.TypeCLI && entityType != types.TypeSkill && entityType != types.TypeMCP && entityType != types.TypeAgent {
		return nil, fmt.Errorf("invalid entity type: %q", entityType)
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 256 || strings.ContainsAny(name, "\r\n") {
		return nil, fmt.Errorf("invalid entity name")
	}
	unlock, err := lockDatabase(path)
	if err != nil {
		return nil, err
	}
	defer unlock()
	db, err := Load(path)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%s:%s", entityType, name)
	now := float64(time.Now().Unix())
	rec := db.Entities[key]
	if rec.CallCount == 0 {
		rec.FirstSeen = now
	}
	rec.Name, rec.Type = key, string(entityType)
	rec.CallCount++
	if source != "" {
		rec.VerifiedCalls++
		rec.LastSource = source
	}
	rec.LastSeen, rec.DaysSinceLast = now, 0
	if rec.CallCount >= 10 {
		rec.Tier = string(types.TierHot)
	} else {
		rec.Tier = string(types.TierWarm)
	}
	db.Entities[key] = rec
	if err = saveUnlocked(path, db); err != nil {
		return nil, err
	}
	return &rec, nil
}

func GetRecord(db *types.TelemetryDB, entityType types.EntityType, name string) (types.TelemetryRecord, bool) {
	if db == nil {
		return types.TelemetryRecord{}, false
	}
	record, exists := db.Entities[fmt.Sprintf("%s:%s", entityType, name)]
	if exists {
		return record, true
	}
	// Legacy imports sometimes used unprefixed names. Require a matching type.
	record, exists = db.Entities[name]
	return record, exists && record.Type == string(entityType)
}
