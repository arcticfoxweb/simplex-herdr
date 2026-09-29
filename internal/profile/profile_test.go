package profile

import "testing"

func TestAttachPaneOnlyWhenEmpty(t *testing.T) {
	t.Setenv("SIMPLEX_HOME", t.TempDir())
	p, err := For("alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Ensure(); err != nil {
		t.Fatal(err)
	}
	wrote, err := p.AttachPane("w1:p1")
	if err != nil || !wrote {
		t.Fatalf("first attach wrote=%v err=%v", wrote, err)
	}
	m := p.LoadMeta()
	if m.Pane != "w1:p1" || m.Tmux != "" || m.Target() != "w1:p1" {
		t.Fatalf("meta = %+v", m)
	}
	wrote, err = p.AttachPane("w9:p9")
	if err != nil || wrote {
		t.Fatalf("second attach wrote=%v err=%v", wrote, err)
	}
	if got := p.LoadMeta().Pane; got != "w1:p1" {
		t.Fatalf("pane = %q", got)
	}
	wrote, err = p.AttachPane("")
	if err != nil || wrote {
		t.Fatalf("empty attach wrote=%v err=%v", wrote, err)
	}
}

func TestAttachPaneLeavesLegacyTmux(t *testing.T) {
	t.Setenv("SIMPLEX_HOME", t.TempDir())
	p, err := For("bob")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := p.SaveMeta(Meta{Name: "bob", Tmux: "old"}); err != nil {
		t.Fatal(err)
	}
	wrote, err := p.AttachPane("w1:p1")
	if err != nil || wrote {
		t.Fatalf("wrote=%v err=%v", wrote, err)
	}
	m := p.LoadMeta()
	if m.Tmux != "old" || m.Pane != "" || m.Target() != "old" {
		t.Fatalf("meta = %+v", m)
	}
}
