package dashboard

import (
	"path/filepath"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

// CleanupService exposes exactly the same guarded filesystem operations to
// local CLI agents and the dashboard. It does not grant elevated OS rights.
type CleanupService struct {
	manager *cleanupManager
}

func NewCleanupService(home string) *CleanupService {
	store := newStore(filepath.Join(home, ".ash", "db", "dashboard.json"), home)
	return &CleanupService{manager: newCleanupManager(store)}
}

func (c *CleanupService) Plan(item types.Entity) (CleanupPlan, error) {
	return c.manager.Plan(item)
}

func (c *CleanupService) Remove(item types.Entity, fingerprint, confirmation string) (TrashRecord, error) {
	return c.manager.Remove(item, fingerprint, confirmation)
}

func (c *CleanupService) Restore(trashID string) error {
	return c.manager.Restore(trashID)
}

func (c *CleanupService) Purge(trashID, confirmation string) error {
	return c.manager.Purge(trashID, confirmation)
}

func (c *CleanupService) Trash() (map[string]TrashRecord, error) {
	state, err := c.manager.store.View()
	if err != nil {
		return nil, err
	}
	return state.Trash, nil
}
