package gateway

import "testing"

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
