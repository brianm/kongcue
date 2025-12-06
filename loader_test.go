package kongcue_test

import (
	"os"
	"path/filepath"
	"testing"

	"cuelang.org/go/cue"
	"github.com/brianm/kong-cue"
)

// testConfig is a sample config struct for testing
type testConfig struct {
	Insecure bool              `json:"insecure"`
	Verbose  int               `json:"verbose"`
	LogFile  string            `json:"log_file"`
	Agent    *testAgentConfig  `json:"agent,omitempty"`
}

type testAgentConfig struct {
	Match []string `json:"match"`
	CaURL string   `json:"ca_url"`
	Auth  string   `json:"auth"`
}

func TestLoadFromFile_YAML(t *testing.T) {
	yaml := `
insecure: true
verbose: 2
log_file: "/var/log/test.log"

agent:
  match:
    - "*.example.com"
    - "*.internal"
  ca_url: "https://ca.example.com"
  auth: "my-auth-command"
`

	tempFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tempFile, []byte(yaml), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	cfg, err := kongcue.LoadFromFile[testConfig](tempFile)
	if err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if !cfg.Insecure {
		t.Error("expected insecure to be true")
	}

	if cfg.Verbose != 2 {
		t.Errorf("expected verbose 2, got %d", cfg.Verbose)
	}

	if cfg.LogFile != "/var/log/test.log" {
		t.Errorf("unexpected log_file: %s", cfg.LogFile)
	}

	if cfg.Agent == nil {
		t.Fatal("agent is nil")
	}

	if len(cfg.Agent.Match) != 2 {
		t.Errorf("expected 2 match patterns, got %d", len(cfg.Agent.Match))
	}

	if cfg.Agent.CaURL != "https://ca.example.com" {
		t.Errorf("unexpected ca_url: %s", cfg.Agent.CaURL)
	}
}

func TestLoadFromFile_JSON(t *testing.T) {
	json := `{
  "insecure": true,
  "agent": {
    "match": ["*.example.com"],
    "ca_url": "https://ca.example.com"
  }
}`

	tempFile := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(tempFile, []byte(json), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	cfg, err := kongcue.LoadFromFile[testConfig](tempFile)
	if err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if !cfg.Insecure {
		t.Error("expected insecure to be true")
	}

	if cfg.Agent == nil {
		t.Fatal("agent is nil")
	}

	if len(cfg.Agent.Match) != 1 {
		t.Errorf("expected 1 match pattern, got %d", len(cfg.Agent.Match))
	}
}

func TestLoadFromFile_NonexistentFile(t *testing.T) {
	_, err := kongcue.LoadFromFile[testConfig]("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadValue_DirectPathLookup(t *testing.T) {
	yaml := `
insecure: true
verbose: 2

agent:
  match:
    - "*.example.com"
    - "*.internal"
  ca_url: "https://ca.example.com"
`

	tempFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tempFile, []byte(yaml), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	val, err := kongcue.LoadValue(tempFile)
	if err != nil {
		t.Fatalf("failed to load value: %v", err)
	}

	// Test direct path lookups (this is how the kong resolver uses it)
	tests := []struct {
		path     string
		wantStr  string
		wantBool bool
		wantInt  int64
		isBool   bool
		isInt    bool
	}{
		{"insecure", "", true, 0, true, false},
		{"verbose", "", false, 2, false, true},
		{"agent.ca_url", "https://ca.example.com", false, 0, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			v := val.LookupPath(cue.ParsePath(tt.path))
			if !v.Exists() {
				t.Errorf("path %s does not exist", tt.path)
				return
			}

			if tt.isBool {
				b, err := v.Bool()
				if err != nil {
					t.Errorf("failed to get bool: %v", err)
				} else if b != tt.wantBool {
					t.Errorf("got %v, want %v", b, tt.wantBool)
				}
			} else if tt.isInt {
				i, err := v.Int64()
				if err != nil {
					t.Errorf("failed to get int: %v", err)
				} else if i != tt.wantInt {
					t.Errorf("got %d, want %d", i, tt.wantInt)
				}
			} else {
				s, err := v.String()
				if err != nil {
					t.Errorf("failed to get string: %v", err)
				} else if s != tt.wantStr {
					t.Errorf("got %q, want %q", s, tt.wantStr)
				}
			}
		})
	}

	// Test list lookup
	matchVal := val.LookupPath(cue.ParsePath("agent.match"))
	if !matchVal.Exists() {
		t.Fatal("agent.match does not exist")
	}

	iter, err := matchVal.List()
	if err != nil {
		t.Fatalf("failed to get list: %v", err)
	}

	var matches []string
	for iter.Next() {
		s, _ := iter.Value().String()
		matches = append(matches, s)
	}

	if len(matches) != 2 {
		t.Errorf("expected 2 matches, got %d", len(matches))
	}
	if matches[0] != "*.example.com" {
		t.Errorf("first match: got %q, want %q", matches[0], "*.example.com")
	}
}

func TestLoadAndUnifyPaths_SingleYAML(t *testing.T) {
	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(yamlFile, []byte(`
agent:
  ca_url: "https://ca.example.com"
`), 0644); err != nil {
		t.Fatal(err)
	}

	val, err := kongcue.LoadAndUnifyPaths([]string{yamlFile})
	if err != nil {
		t.Fatalf("LoadAndUnifyPaths failed: %v", err)
	}

	caURL, err := val.LookupPath(cue.ParsePath("agent.ca_url")).String()
	if err != nil {
		t.Fatalf("failed to get agent.ca_url: %v", err)
	}
	if caURL != "https://ca.example.com" {
		t.Errorf("expected https://ca.example.com, got %s", caURL)
	}
}

func TestLoadAndUnifyPaths_SingleCUE(t *testing.T) {
	dir := t.TempDir()
	cueFile := filepath.Join(dir, "config.cue")
	if err := os.WriteFile(cueFile, []byte(`
agent: {
	ca_url: "https://ca.example.com"
	port: 8080
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	val, err := kongcue.LoadAndUnifyPaths([]string{cueFile})
	if err != nil {
		t.Fatalf("LoadAndUnifyPaths failed: %v", err)
	}

	caURL, err := val.LookupPath(cue.ParsePath("agent.ca_url")).String()
	if err != nil {
		t.Fatalf("failed to get agent.ca_url: %v", err)
	}
	if caURL != "https://ca.example.com" {
		t.Errorf("expected https://ca.example.com, got %s", caURL)
	}

	port, err := val.LookupPath(cue.ParsePath("agent.port")).Int64()
	if err != nil {
		t.Fatalf("failed to get agent.port: %v", err)
	}
	if port != 8080 {
		t.Errorf("expected 8080, got %d", port)
	}
}

func TestLoadAndUnifyPaths_MultipleFilesCompatible(t *testing.T) {
	dir := t.TempDir()

	// First file with ca_url
	file1 := filepath.Join(dir, "base.yaml")
	if err := os.WriteFile(file1, []byte(`
agent:
  ca_url: "https://ca.example.com"
`), 0644); err != nil {
		t.Fatal(err)
	}

	// Second file with match patterns (different field, compatible)
	file2 := filepath.Join(dir, "matches.yaml")
	if err := os.WriteFile(file2, []byte(`
agent:
  match:
    - "*.example.com"
`), 0644); err != nil {
		t.Fatal(err)
	}

	val, err := kongcue.LoadAndUnifyPaths([]string{file1, file2})
	if err != nil {
		t.Fatalf("LoadAndUnifyPaths failed: %v", err)
	}

	// Check ca_url from first file
	caURL, err := val.LookupPath(cue.ParsePath("agent.ca_url")).String()
	if err != nil {
		t.Fatalf("failed to get agent.ca_url: %v", err)
	}
	if caURL != "https://ca.example.com" {
		t.Errorf("expected https://ca.example.com, got %s", caURL)
	}

	// Check match from second file
	matchVal := val.LookupPath(cue.ParsePath("agent.match"))
	iter, err := matchVal.List()
	if err != nil {
		t.Fatalf("failed to get agent.match as list: %v", err)
	}

	var matches []string
	for iter.Next() {
		s, _ := iter.Value().String()
		matches = append(matches, s)
	}
	if len(matches) != 1 || matches[0] != "*.example.com" {
		t.Errorf("expected [*.example.com], got %v", matches)
	}
}

func TestLoadAndUnifyPaths_ConflictingValues(t *testing.T) {
	dir := t.TempDir()

	// First file with ca_url
	file1 := filepath.Join(dir, "base.yaml")
	if err := os.WriteFile(file1, []byte(`
agent:
  ca_url: "https://ca1.example.com"
`), 0644); err != nil {
		t.Fatal(err)
	}

	// Second file with different ca_url (conflict!)
	file2 := filepath.Join(dir, "override.yaml")
	if err := os.WriteFile(file2, []byte(`
agent:
  ca_url: "https://ca2.example.com"
`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := kongcue.LoadAndUnifyPaths([]string{file1, file2})
	if err == nil {
		t.Fatal("expected error for conflicting values, got nil")
	}
}

func TestLoadAndUnifyPaths_GlobPattern(t *testing.T) {
	dir := t.TempDir()
	confDir := filepath.Join(dir, "config.d")
	if err := os.MkdirAll(confDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create multiple yaml files
	if err := os.WriteFile(filepath.Join(confDir, "a.yaml"), []byte(`
settings:
  a: true
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(confDir, "b.yaml"), []byte(`
settings:
  b: true
`), 0644); err != nil {
		t.Fatal(err)
	}

	val, err := kongcue.LoadAndUnifyPaths([]string{filepath.Join(confDir, "*.yaml")})
	if err != nil {
		t.Fatalf("LoadAndUnifyPaths failed: %v", err)
	}

	// Check both values are present
	a, err := val.LookupPath(cue.ParsePath("settings.a")).Bool()
	if err != nil {
		t.Fatalf("failed to get settings.a: %v", err)
	}
	if !a {
		t.Error("expected settings.a to be true")
	}

	b, err := val.LookupPath(cue.ParsePath("settings.b")).Bool()
	if err != nil {
		t.Fatalf("failed to get settings.b: %v", err)
	}
	if !b {
		t.Error("expected settings.b to be true")
	}
}

func TestLoadAndUnifyPaths_MissingFilesSkipped(t *testing.T) {
	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "exists.yaml")
	if err := os.WriteFile(yamlFile, []byte(`
key: value
`), 0644); err != nil {
		t.Fatal(err)
	}

	// Include a non-existent file in the patterns
	val, err := kongcue.LoadAndUnifyPaths([]string{
		filepath.Join(dir, "does-not-exist.yaml"),
		yamlFile,
	})
	if err != nil {
		t.Fatalf("LoadAndUnifyPaths failed: %v", err)
	}

	// The existing file's value should be present
	key, err := val.LookupPath(cue.ParsePath("key")).String()
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}
	if key != "value" {
		t.Errorf("expected 'value', got %s", key)
	}
}

func TestLoadAndUnifyPaths_EmptyResult(t *testing.T) {
	dir := t.TempDir()

	// No files exist
	val, err := kongcue.LoadAndUnifyPaths([]string{
		filepath.Join(dir, "does-not-exist.yaml"),
	})
	if err != nil {
		t.Fatalf("LoadAndUnifyPaths failed: %v", err)
	}

	// Should return an empty object, not an error
	if !val.Exists() {
		t.Error("expected value to exist (empty object)")
	}
}

func TestLoadAndUnifyPaths_MixedFileTypes(t *testing.T) {
	dir := t.TempDir()

	// YAML file
	yamlFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(yamlFile, []byte(`
from_yaml: true
`), 0644); err != nil {
		t.Fatal(err)
	}

	// CUE file
	cueFile := filepath.Join(dir, "config.cue")
	if err := os.WriteFile(cueFile, []byte(`
from_cue: true
`), 0644); err != nil {
		t.Fatal(err)
	}

	// JSON file
	jsonFile := filepath.Join(dir, "config.json")
	if err := os.WriteFile(jsonFile, []byte(`{"from_json": true}`), 0644); err != nil {
		t.Fatal(err)
	}

	val, err := kongcue.LoadAndUnifyPaths([]string{yamlFile, cueFile, jsonFile})
	if err != nil {
		t.Fatalf("LoadAndUnifyPaths failed: %v", err)
	}

	// Check all values are present
	fromYAML, _ := val.LookupPath(cue.ParsePath("from_yaml")).Bool()
	if !fromYAML {
		t.Error("expected from_yaml to be true")
	}

	fromCUE, _ := val.LookupPath(cue.ParsePath("from_cue")).Bool()
	if !fromCUE {
		t.Error("expected from_cue to be true")
	}

	fromJSON, _ := val.LookupPath(cue.ParsePath("from_json")).Bool()
	if !fromJSON {
		t.Error("expected from_json to be true")
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"empty", "", "", false},
		{"no tilde", "/some/path", "/some/path", false},
		{"just tilde", "~", home, false},
		{"tilde with path", "~/foo/bar", filepath.Join(home, "foo/bar"), false},
		{"tilde in middle", "/some/~path", "/some/~path", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := kongcue.ExpandPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExpandPath() = %v, want %v", got, tt.want)
			}
		})
	}
}
