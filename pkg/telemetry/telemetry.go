package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

var (
	dbLock sync.RWMutex
)

// DefaultDBPath returns ~/.ash/db/telemetry.json
func DefaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/telemetry.json"
	}
	return filepath.Join(home, ".ash", "db", "telemetry.json")
}

// Load reads the telemetry database from disk.
func Load(path string) (*types.TelemetryDB, error) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	if path == "" {
		path = DefaultDBPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &types.TelemetryDB{
				Version:       "1.0.0",
				LastUpdated:   float64(time.Now().Unix()),
				TotalEntities: 0,
				Entities:      make(map[string]types.TelemetryRecord),
			}, nil
		}
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

// Save writes the telemetry database to disk atomically.
func Save(path string, db *types.TelemetryDB) error {
	dbLock.Lock()
	defer dbLock.Unlock()

	if path == "" {
		path = DefaultDBPath()
	}

	db.LastUpdated = float64(time.Now().Unix())
	db.TotalEntities = len(db.Entities)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating db dir: %w", err)
	}

	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling telemetry db: %w", err)
	}

	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("writing tmp db: %w", err)
	}

	return os.Rename(tmpFile, path)
}

// RecordInvocation records +1 call for an entity name and type.
func RecordInvocation(path string, entityType types.EntityType, name string) (*types.TelemetryRecord, error) {
	db, err := Load(path)
	if err != nil {
		return nil, err
	}

	key := fmt.Sprintf("%s:%s", entityType, name)
	now := float64(time.Now().Unix())

	rec, exists := db.Entities[key]
	if !exists {
		rec = types.TelemetryRecord{
			Name:          key,
			Type:          string(entityType),
			CallCount:     1,
			FirstSeen:     now,
			LastSeen:      now,
			DaysSinceLast: 0,
			Tier:          string(types.TierWarm),
		}
	} else {
		rec.CallCount++
		rec.LastSeen = now
		rec.DaysSinceLast = 0
		if rec.CallCount >= 10 {
			rec.Tier = string(types.TierHot)
		} else {
			rec.Tier = string(types.TierWarm)
		}
	}

	db.Entities[key] = rec
	if err := Save(path, db); err != nil {
		return nil, err
	}

	return &rec, nil
}

// GetRecord returns telemetry for an entity if found.
func GetRecord(db *types.TelemetryDB, entityType types.EntityType, name string) (types.TelemetryRecord, bool) {
	key := fmt.Sprintf("%s:%s", entityType, name)
	rec, ok := db.Entities[key]
	if ok {
		return rec, true
	}
	// Fallback check by raw name
	rec, ok = db.Entities[name]
	return rec, ok
}
