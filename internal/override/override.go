package override

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

const FileName = ".lintmax-override.yml"

type Forbid struct {
	Pattern string `yaml:"pattern"`
	Msg     string `yaml:"msg"`
}

type Spec struct {
	ExhaustructInclude []string `yaml:"exhaustructInclude"`
	ForbidigoForbid    []Forbid `yaml:"forbidigoForbid"`
}

func Load(dir string) (Spec, error) {
	path := filepath.Join(dir, FileName)
	body, err := os.ReadFile(path) //nolint:gosec // consumer override in the linted repo root
	if err != nil {
		if os.IsNotExist(err) {
			return Spec{}, nil
		}
		return Spec{}, fmt.Errorf("read %s: %w", FileName, err)
	}
	var spec Spec
	uErr := yaml.Unmarshal(body, &spec)
	if uErr != nil {
		return Spec{}, fmt.Errorf("parse %s: %w", FileName, uErr)
	}
	return spec, nil
}

func (s Spec) Empty() bool {
	return len(s.ExhaustructInclude) == 0 && len(s.ForbidigoForbid) == 0
}

const (
	disableLine    = "    - exhaustruct"
	forbidAnchor   = "      forbid: # LINTMAX_FORBIDIGO_FORBID"
	settingsAnchor = "  settings:"
)

func Apply(cfg string, spec Spec) string {
	cfg = applyExhaustruct(cfg, spec.ExhaustructInclude)
	cfg = applyForbidigo(cfg, spec.ForbidigoForbid)
	return cfg
}

func applyExhaustruct(cfg string, include []string) string {
	if len(include) == 0 {
		return cfg
	}
	cfg = dropDisableLine(cfg)
	var b strings.Builder
	b.WriteString(settingsAnchor + "\n")
	b.WriteString("    exhaustruct:\n")
	b.WriteString("      include:\n")
	for _, p := range include {
		fmt.Fprintf(&b, "        - %s\n", yamlScalar(p))
	}
	return strings.Replace(cfg, settingsAnchor+"\n", b.String(), 1)
}

func dropDisableLine(cfg string) string {
	lines := strings.Split(cfg, "\n")
	kept := make([]string, 0, len(lines))
	for _, ln := range lines {
		if ln == disableLine || strings.HasPrefix(ln, disableLine+" ") {
			continue
		}
		kept = append(kept, ln)
	}
	return strings.Join(kept, "\n")
}

func applyForbidigo(cfg string, forbid []Forbid) string {
	if len(forbid) == 0 {
		return cfg
	}
	var b strings.Builder
	b.WriteString(forbidAnchor + "\n")
	for _, f := range forbid {
		fmt.Fprintf(&b, "        - pattern: %s\n", yamlScalar(f.Pattern))
		if f.Msg != "" {
			fmt.Fprintf(&b, "          msg: %s\n", yamlScalar(f.Msg))
		}
	}
	return strings.Replace(cfg, forbidAnchor+"\n", b.String(), 1)
}

func yamlScalar(s string) string {
	out, err := yaml.Marshal(s)
	if err != nil {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	return strings.TrimRight(string(out), "\n")
}
