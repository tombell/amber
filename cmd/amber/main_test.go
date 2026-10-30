package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRejectsEmptyTargetFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--server=", "list"},
		{"-S", "", "list"},
		{"list", "--client="},
		{"show", "-C", "", "demo"},
	} {
		var stdout, stderr bytes.Buffer
		if err := run(args, &stdout, &stderr); err == nil {
			t.Errorf("run(%q) error = nil", args)
		}
		if !strings.Contains(stderr.String(), "requires a non-empty") {
			t.Errorf("run(%q) stderr = %q", args, stderr.String())
		}
	}
}
