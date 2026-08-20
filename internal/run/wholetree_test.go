package run_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestEveryWholeTreeStageHonoursTheOptIn(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("run.go")
	if err != nil {
		t.Fatalf("reading run.go: %v", err)
	}
	sites := regexp.MustCompile(`(\w+)\.Gate\(g\.ctx, "\."`).FindAllStringSubmatch(string(body), -1)
	if len(sites) == 0 {
		t.Fatal("the derivation found no whole-tree stage, so this guard judges nothing")
	}
	for _, site := range sites {
		pkg := site[1]
		//nolint:gosec // reason: the path is assembled from locally discovered repository package names
		src, readErr := os.ReadFile(filepath.Join("..", pkg, pkg+".go"))
		if readErr != nil {
			t.Fatalf("reading the %s stage: %v", pkg, readErr)
		}
		if !strings.Contains(string(src), "WholeTreeRequested()") {
			t.Errorf("the %s stage walks the whole tree and never asks WholeTreeRequested, so it judges "+
				"every file in a consumer whether or not that consumer has ground its baseline", pkg)
		}
	}
}
