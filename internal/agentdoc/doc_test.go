package agentdoc

import (
	"os"
	"strings"
	"testing"
)

func TestSkillMatchesEmbeddedInstructions(t *testing.T) {
	prompt, err := os.ReadFile("../../AGENT_PROMPT.md")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile("../../herdr-plugin/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != Text {
		t.Fatal("herdr-plugin/SKILL.md and embedded instructions.md differ")
	}
	if !strings.Contains(string(prompt), string(b)) || !strings.Contains(string(prompt), "herdr plugin link") {
		t.Fatal("AGENT_PROMPT.md must contain the link step and the full skill")
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
