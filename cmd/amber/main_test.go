package main

import (
	"bytes"
	"errors"
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
		if err := run(args, &stdout, &stderr); !errors.Is(err, errUsage) {
			t.Errorf("run(%q) error = %v, want errUsage", args, err)
		}
		if strings.Count(stderr.String(), "requires a non-empty") != 1 {
			t.Errorf("run(%q) stderr = %q", args, stderr.String())
		}
	}
}
