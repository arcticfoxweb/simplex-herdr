package install

import "testing"

func TestDLLNotFoundCode(t *testing.T) {
	if !dllNotFound(0xC0000135) || !dllNotFound(-1073741515) {
		t.Fatal("0xc0000135 should be recognized")
	}
	if dllNotFound(0) || dllNotFound(1) {
		t.Fatal("other exit codes are not a missing DLL")
	}
}
