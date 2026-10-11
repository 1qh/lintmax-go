package config_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/1qh/lintmax-go/internal/config"
)

func TestModernizeKeepsRewritesOtherGateToolsCannotAccept(t *testing.T) {
	t.Parallel()
	block := regexp.MustCompile(`(?s)\n {4}modernize:\n(.*?)\n {4}[a-z]`)
	match := block.FindStringSubmatch(string(config.GolangCI))
	if match == nil {
		t.Fatal("the modernize settings block is missing")
	}
	for _, analyzer := range []string{"newexpr", "embedlit"} {
		if !strings.Contains(match[1]+"\n", "        - "+analyzer+"\n") {
			t.Fatalf("modernize must disable %s, whose rewrite another gate tool refuses:\n%s", analyzer, match[1])
		}
	}
}
