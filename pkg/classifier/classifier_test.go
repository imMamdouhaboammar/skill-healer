package classifier

import (
	"testing"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

func TestUnknownNameIsNotMatchedInsideUnrelatedWord(t *testing.T) {
	if got := ClassifyName("builder"); got != types.DomainGeneral {
		t.Fatalf("substring matched unrelated UI token: %q", got)
	}
}

func TestMeaningfulDescriptionTakesPrecedence(t *testing.T) {
	result := ClassifyDetailed("misc-helper", "Checks leaked secrets, credential exposure and access control errors")
	if result.Domain != types.DomainSecurity {
		t.Fatalf("security purpose was ignored: %+v", result)
	}
	if result.Confidence == 0 || result.Basis == "" {
		t.Fatalf("missing classification evidence: %+v", result)
	}
}
