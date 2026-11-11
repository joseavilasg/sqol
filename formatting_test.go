package main

import (
	"os/exec"
	"strings"
	"testing"
)

// TestTheTreeIsGofmtClean catches a file an editor saved unformatted. It runs
// the check through go test so every platform reports it the same way, without
// a shell that has to parse the output.
func TestTheTreeIsGofmtClean(t *testing.T) {
	out, err := exec.Command("gofmt", "-l", ".").Output()
	if err != nil {
		t.Fatalf("gofmt: %v", err)
	}

	if files := strings.TrimSpace(string(out)); files != "" {
		t.Errorf("gofmt would rewrite these files:\n%s", files)
	}
}
