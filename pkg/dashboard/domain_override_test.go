package dashboard

import (
	"path/filepath"
	"testing"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

func TestDomainOverridePersistsAndRejectsInvalidDomains(t *testing.T) {
	home := t.TempDir()
	store := newStore(filepath.Join(home, "state.json"), home)
	if err := store.SetTool("skill:auditor", ToolPreference{Domain: types.Domain("Unknown invented domain")}); err == nil {
		t.Fatal("accepted an unsupported domain override")
	}
	if err := store.SetTool("skill:auditor", ToolPreference{Domain: types.DomainSecurity}); err != nil {
		t.Fatal(err)
	}
	saved, err := store.View()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Tools["skill:auditor"].Domain != types.DomainSecurity {
		t.Fatal("manual classification was not persisted")
	}
}
