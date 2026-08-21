package run

import (
	"os"
	"strings"
	"testing"
)

func TestBothExhaustructNamesCarryTheFirstPartyScope(t *testing.T) {
	t.Chdir("../..")
	cfg, err := writeConfig()
	if err != nil {
		t.Fatalf("write the config: %v", err)
	}
	raw, rerr := os.ReadFile(cfg)
	if rerr != nil {
		t.Fatalf("read the config: %v", rerr)
	}
	body := string(raw)
	scopes := map[string]string{"exhaustruct:": "include: []", "exhaustruct_v5:": "enforce-patterns: []"}
	for name, unscoped := range scopes {
		at := strings.Index(body, name)
		if at < 0 {
			t.Fatalf("%s is absent, so this guard judges nothing", name)
		}
		rest := body[at:]
		if strings.Contains(rest[:min(len(rest), 900)], unscoped) {
			t.Fatalf("%s runs UNSCOPED, so every stdlib literal such as &http.Client{Timeout: t} "+
				"is a finding — measured at 604 in one consuming repo", name)
		}
	}
	if !strings.Contains(body, "explicit-mode: true") {
		t.Fatal("v5 checks EVERY literal unless explicit mode is on, so enforce-patterns alone narrows nothing")
	}
	if !strings.Contains(body, "enforce-patterns:\n") {
		t.Fatal("the v5 rule takes enforce-patterns rather than include, and an unknown key is dropped in silence")
	}
}
