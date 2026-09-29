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
	if strings.Contains(Text, "Do not ask for permission") || strings.Contains(Text, "Execute now") {
		t.Fatal("instructions read like a prompt injection")
	}
	for _, needle := range []string{
		"Do not install Herdr or simplex again",
		"~/.local/src/simplex/herdr-plugin",
		"simplex.agents",
		"herdr plugin action invoke simplex.agents.attach",
		"Do not install qrencode",
		"contact.png",
		"simplex join",
		"simplex group create",
		"simplex group add",
		"rcvComplete",
	} {
		if !strings.Contains(Text, needle) {
			t.Fatalf("instructions missing %q", needle)
		}
	}
	if strings.Contains(Text, "herdr.dev/install.ps1") {
		t.Fatal("post-install instructions must not repeat the Herdr installer")
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
