package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	dirPerm  = 0o755
	filePerm = 0o600
	appDir   = "lintmax-go"
	fileName = "versions.json"
)

type State struct {
	Versions map[string]string `json:"versions"`
}

func path() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("cache dir: %w", err)
	}
	return filepath.Join(dir, appDir, fileName), nil
}

func Load() State {
	empty := State{Versions: map[string]string{}}
	p, err := path()
	if err != nil {
		return empty
	}
	raw, err := os.ReadFile(p) //nolint:gosec // path derived from os.UserCacheDir, not user input
	if err != nil {
		return empty
	}
	var loaded State
	err = json.Unmarshal(raw, &loaded)
	if err != nil || loaded.Versions == nil {
		return empty
	}
	return loaded
}

func (s State) Save() error {
	cachePath, err := path()
	if err != nil {
		return err
	}
	err = os.MkdirAll(filepath.Dir(cachePath), dirPerm)
	if err != nil {
		return fmt.Errorf("mkdir cache: %w", err)
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	err = os.WriteFile(cachePath, raw, filePerm)
	if err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}
