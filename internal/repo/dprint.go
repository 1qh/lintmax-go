package repo

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const host = "https://plugins.dprint.dev/"

const (
	fetchTimeout = 15 * time.Second
	versionParts = 3
)

func Seed() []string {
	return []string{
		host + "json-0.23.0.wasm",
		host + "markdown-0.22.1.wasm",
		host + "toml-0.7.0.wasm",
		host + "dockerfile-0.4.1.wasm",
		host + "g-plane/pretty_yaml-v0.6.0.wasm",
		host + "g-plane/malva-v0.16.0.wasm",
		host + "g-plane/markup_fmt-v0.27.3.wasm",
		host + "g-plane/pretty_graphql-v0.2.3.wasm",
		host + "bartlomieju/lax-sql-0.3.0.wasm",
	}
}

func PluginName(file string) string {
	cut := strings.LastIndexAny(file, "-@")
	if cut < 0 {
		return file
	}
	parts := strings.Split(strings.TrimPrefix(file[cut+1:], "v"), ".")
	if len(parts) != versionParts {
		return file
	}
	for _, part := range parts {
		if part == "" || strings.TrimLeft(part, "0123456789") != "" {
			return file
		}
	}
	return file[:cut]
}

func PluginPath(pinned string) string {
	tail, ok := strings.CutPrefix(pinned, host)
	if !ok {
		return ""
	}
	dot := strings.LastIndex(tail, ".")
	if dot < 0 {
		return ""
	}
	file := tail[:dot]
	owner, rest, split := strings.Cut(file, "/")
	if split {
		return owner + "/" + PluginName(rest)
	}
	return "dprint/" + PluginName(file)
}

func Latest(ctx context.Context, seed []string) []string {
	out := make([]string, len(seed))
	for index, pinned := range seed {
		out[index] = pinned
		path := PluginPath(pinned)
		if path == "" {
			continue
		}
		if url := latestURL(ctx, path); url != "" {
			out[index] = url
		}
	}
	return out
}

func latestURL(ctx context.Context, path string) string {
	timed, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(timed, http.MethodGet, host+path+"/latest.json", http.NoBody)
	if err != nil {
		return ""
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp == nil || resp.Body == nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // nothing actionable on a failed close
	var body struct {
		URL string `json:"url"`
	}
	decodeErr := json.NewDecoder(resp.Body).Decode(&body)
	if decodeErr != nil {
		return ""
	}
	if !strings.HasPrefix(body.URL, host) {
		return ""
	}
	return body.URL
}
