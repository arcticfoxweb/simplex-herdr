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
	if string(prompt) != string(b) {
		t.Fatal("AGENT_PROMPT.md must be the same file as herdr-plugin/SKILL.md")
	}
	if !strings.Contains(Text, "https://raw.githubusercontent.com/arcticfoxweb/simplex-herdr/main/AGENT_PROMPT.md") {
		t.Fatal("built-in instructions are missing the agent prompt link")
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
