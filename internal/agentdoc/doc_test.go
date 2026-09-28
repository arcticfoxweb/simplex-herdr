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
	for _, needle := range []string{
		"msgContent.type",
		"file row",
		"/_send <str(sendRef)>[ live=on][ ttl=<ttl>][ sign=on] json <json(composedMessages)>",
		"APINewGroup",
		"NewChatItems",
		"RcvFileComplete",
	} {
		if !strings.Contains(Text, needle) {
			t.Fatalf("instructions missing %q", needle)
		}
	}
}
