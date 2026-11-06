package dashboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

type BulkItem struct {
	ID          string `json:"id"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type BulkRemoveRequest struct {
	Items        []BulkItem `json:"items"`
	Confirmation string     `json:"confirmation"`
}

type BulkIDsRequest struct {
	IDs          []string `json:"ids"`
	Confirmation string   `json:"confirmation,omitempty"`
}

type BulkActionResult struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Status  string `json:"status"`
	TrashID string `json:"trash_id,omitempty"`
	Error   string `json:"error,omitempty"`
}

type BulkResult struct {
	Completed int                `json:"completed"`
	Failed    int                `json:"failed"`
	Results   []BulkActionResult `json:"results"`
}

func bulkPhrase(count int) string {
	if count == 1 {
		return "REMOVE 1 TOOL"
	}
	return "REMOVE " + strconv.Itoa(count) + " TOOLS"
}

func purgePhrase(count int) string {
	if count == 1 {
		return "DELETE 1 RECOVERY COPY"
	}
	return "DELETE " + strconv.Itoa(count) + " RECOVERY COPIES"
}

func validateBulkIDs(ids []string) error {
	if len(ids) == 0 {
		return ErrUnsafe
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || len(id) > 500 || seen[id] {
			return ErrUnsafe
		}
		seen[id] = true
	}
	return nil
}

func lookupPool(pool *types.Pool, id string) (types.Entity, bool) {
	for _, group := range []map[string]types.Entity{pool.Skills, pool.MCPs, pool.CLIs, pool.Agents} {
		for _, item := range group {
			if item.ID == id {
				return item, true
			}
		}
	}
	return types.Entity{}, false
}

func bulkPlans(pool *types.Pool, manager *cleanupManager, ids []string) ([]CleanupPlan, error) {
	if err := validateBulkIDs(ids); err != nil {
		return nil, err
	}
	plans := make([]CleanupPlan, 0, len(ids))
	for _, id := range ids {
		item, ok := lookupPool(pool, id)
		if !ok {
			plans = append(plans, CleanupPlan{ID: id, Reason: "Tool is no longer in the discovered inventory"})
			continue
		}
		plan, err := manager.Plan(item)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func bulkRemove(pool *types.Pool, manager *cleanupManager, request BulkRemoveRequest) (BulkResult, error) {
	ids := make([]string, 0, len(request.Items))
	for _, entry := range request.Items {
		ids = append(ids, entry.ID)
	}
	if err := validateBulkIDs(ids); err != nil {
		return BulkResult{}, err
	}
	if request.Confirmation != bulkPhrase(len(ids)) {
		return BulkResult{}, ErrUnsafe
	}
	type verified struct {
		item     types.Entity
		initial  BulkItem
		mcpEntry []byte
	}
	items := make([]verified, 0, len(ids))
	for _, given := range request.Items {
		item, ok := lookupPool(pool, given.ID)
		if !ok {
			return BulkResult{}, ErrNotFound
		}
		plan, err := manager.Plan(item)
		if err != nil {
			return BulkResult{}, err
		}
		if !plan.Allowed {
			return BulkResult{}, ErrUnsafe
		}
		if given.Fingerprint == "" || plan.Fingerprint != given.Fingerprint {
			return BulkResult{}, ErrConflict
		}
		current := verified{item: item, initial: given}
		if plan.Operation == "remove-mcp-entry" {
			_, _, entry, err := readMCP(item.Path, item.Name)
			if err != nil {
				return BulkResult{}, err
			}
			current.mcpEntry = entry
		}
		items = append(items, current)
	}
	result := BulkResult{Results: make([]BulkActionResult, 0, len(items))}
	modifiedConfigs := map[string]bool{}
	for _, current := range items {
		fp := current.initial.Fingerprint
		if modifiedConfigs[current.item.Path] && current.item.Type == types.TypeMCP && current.mcpEntry != nil {
			_, _, now, err := readMCP(current.item.Path, current.item.Name)
			if err != nil || !jsonEquivalent(now, current.mcpEntry) {
				result.Failed++
				result.Results = append(result.Results, BulkActionResult{ID: current.item.ID, Name: current.item.Name, Status: "failed", Error: "MCP entry changed during batch"})
				continue
			}
			plan, err := manager.Plan(current.item)
			if err != nil || !plan.Allowed {
				result.Failed++
				result.Results = append(result.Results, BulkActionResult{ID: current.item.ID, Name: current.item.Name, Status: "failed", Error: "MCP source became unavailable"})
				continue
			}
			fp = plan.Fingerprint
		}
		record, err := manager.Remove(current.item, fp, current.item.Name)
		if err != nil {
			result.Failed++
			result.Results = append(result.Results, BulkActionResult{ID: current.item.ID, Name: current.item.Name, Status: "failed", Error: bulkFailure(err)})
			continue
		}
		result.Completed++
		result.Results = append(result.Results, BulkActionResult{ID: current.item.ID, Name: current.item.Name, Status: "removed", TrashID: record.ID})
		if current.mcpEntry != nil {
			modifiedConfigs[current.item.Path] = true
		}
	}
	return result, nil
}

func jsonEquivalent(a, b []byte) bool {
	var left, right bytes.Buffer
	if json.Compact(&left, a) != nil || json.Compact(&right, b) != nil {
		return false
	}
	return bytes.Equal(left.Bytes(), right.Bytes())
}

func bulkFailure(err error) string {
	switch {
	case errors.Is(err, os.ErrPermission):
		return "macOS permissions denied access to this source"
	case errors.Is(err, ErrConflict):
		return "Source changed since confirmation or restore destination exists"
	case errors.Is(err, ErrNotFound), errors.Is(err, os.ErrNotExist):
		return "Source is missing"
	default:
		return "Operation failed; source was left unchanged where possible"
	}
}

func bulkTrash(manager *cleanupManager, ids []string, operation, confirmation string) (BulkResult, error) {
	if err := validateBulkIDs(ids); err != nil {
		return BulkResult{}, err
	}
	if operation != "restore" && operation != "purge" {
		return BulkResult{}, ErrUnsafe
	}
	if operation == "purge" && confirmation != purgePhrase(len(ids)) {
		return BulkResult{}, ErrUnsafe
	}
	state, err := manager.store.View()
	if err != nil {
		return BulkResult{}, err
	}
	for _, id := range ids {
		if _, exists := state.Trash[id]; !exists {
			return BulkResult{}, ErrNotFound
		}
	}
	result := BulkResult{Results: make([]BulkActionResult, 0, len(ids))}
	for _, id := range ids {
		record := state.Trash[id]
		if operation == "restore" {
			err = manager.Restore(id)
		} else {
			err = manager.Purge(id, "DELETE "+record.Name)
		}
		if err != nil {
			result.Failed++
			result.Results = append(result.Results, BulkActionResult{ID: id, Name: record.Name, Status: "failed", Error: bulkFailure(err)})
			continue
		}
		result.Completed++
		result.Results = append(result.Results, BulkActionResult{ID: id, Name: record.Name, Status: operation + "d"})
	}
	return result, nil
}

// Update the selected user metadata in one state write, without touching tools.
func (s *Store) BulkUpdateTools(ids []string, action string) error {
	if err := validateBulkIDs(ids); err != nil {
		return err
	}
	if action != "pin" && action != "unpin" && action != "hide" && action != "unhide" {
		return ErrUnsafe
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readLocked()
	if err != nil {
		return err
	}
	for _, id := range ids {
		pref := state.Tools[id]
		switch action {
		case "pin":
			pref.Favorite = true
		case "unpin":
			pref.Favorite = false
		case "hide":
			pref.Hidden = true
		case "unhide":
			pref.Hidden = false
		}
		pref.Updated = time.Now().UTC()
		state.Tools[id] = pref
	}
	return s.saveLocked(state)
}
