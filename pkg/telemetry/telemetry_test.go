package telemetry

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

func TestRecordAcrossProcesses(t *testing.T) {
	if os.Getenv("ASH_TEST_WORKER") == "1" {
		db := os.Getenv("ASH_TEST_DB")
		for i := 0; i < 12; i++ {
			if _, err := RecordObservedInvocation(db, types.TypeMCP, "fixture", "test-host"); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	path := filepath.Join(t.TempDir(), "db", "telemetry.json")
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestRecordAcrossProcesses$")
			cmd.Env = append(os.Environ(), "ASH_TEST_WORKER=1", "ASH_TEST_DB="+path)
			errs <- cmd.Run()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("worker failed: %v", err)
		}
	}
	db, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := GetRecord(db, types.TypeMCP, "fixture")
	if !ok || rec.CallCount != 96 || rec.VerifiedCalls != 96 {
		t.Fatalf("lost invocations: %+v", rec)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatalf("telemetry db is exposed to other users: %s", info.Mode())
	}
}

func TestManualUsageIsNotHostVerified(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.json")
	if _, err := RecordInvocation(path, types.TypeSkill, "draft"); err != nil {
		t.Fatal(err)
	}
	rec, err := RecordObservedInvocation(path, types.TypeSkill, "draft", "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if rec.CallCount != 2 || rec.VerifiedCalls != 1 || rec.LastSource != "claude-code" {
		t.Fatalf("incorrect provenance: %+v", rec)
	}
}

func TestInvalidEventsDoNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.json")
	if _, err := RecordObservedInvocation(path, types.TypeSkill, "x", ""); err == nil {
		t.Fatal("accepted unverified source")
	}
	if _, err := RecordInvocation(path, types.EntityType("error"), "x"); err == nil {
		t.Fatal("accepted unknown type")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unexpected db write: %v", err)
	}
}
