package agentdoc

import (
	"os"
	"strings"
	"testing"
)

func TestSkillMatchesEmbeddedInstructions(t *testing.T) {
	b, err := os.ReadFile("../../herdr-plugin/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != Text {
		t.Fatal("herdr-plugin/SKILL.md and embedded instructions.md differ")
	}
	if !strings.Contains(Text, "msgContent.type") || !strings.Contains(Text, "file row") {
		t.Fatal("instructions are missing the picture versus file rule")
	}
}
