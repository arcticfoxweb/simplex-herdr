package gateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAgentStatus(t *testing.T) {
	raw := []byte(`{"id":"cli:agent:get","result":{"agent":{"agent_status":"idle","pane_id":"w1:p1"},"type":"agent_info"}}` + "\n")
	got, err := parseAgentStatus(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != "idle" {
		t.Fatalf("status = %s", got)
	}
	if !agentIdle("idle") || !agentIdle("done") || agentIdle("working") || agentIdle("blocked") {
		t.Fatal("idle classification")
	}
}

func TestValidHerdrPane(t *testing.T) {
	if err := ValidTarget("w1:p1"); err != nil {
		t.Fatal(err)
	}
	if err := ValidTarget("has space"); err == nil {
		t.Fatal("expected reject")
	}
}

func TestRememberHerdrPersistsAbsolutePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIMPLEX_HOME", home)
	bin := filepath.Join(home, "herdr")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_BIN_PATH", bin)
	got := RememberHerdr()
	if got != bin {
		t.Fatalf("RememberHerdr = %q, want %s", got, bin)
	}
	saved, err := os.ReadFile(filepath.Join(home, "herdr.path"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(saved)) != bin {
		t.Fatalf("herdr.path = %q", saved)
	}

	t.Setenv("HERDR_BIN_PATH", filepath.Join(home, "missing"))
	if again := herdrBin(); again != bin {
		t.Fatalf("herdrBin after a bad env = %q, want saved %s", again, bin)
	}
}
