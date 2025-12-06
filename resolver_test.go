package kongcue_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/kong"
	kongcue "github.com/brianm/kongcue"
)

// Test CLI structure for resolver tests
type testCLI struct {
	Verbose int    `short:"v" type:"counter"`
	LogFile string `name:"log-file"`

	Agent testAgentCLI `cmd:"agent"`
}

type testAgentCLI struct {
	CaURL string   `name:"ca-url"`
	Match []string `name:"match"`
	Auth  string   `name:"auth"`
}

func TestBasics(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(configFile, []byte("name: Brian"), 0666)

	var cli struct {
		Name   string         `name:"name" help:"a name" default:"Tom"`
		Config kongcue.Config `name:"config"`
	}

	parser, err := kong.New(&cli)
	if err != nil {
		t.Logf("unexpected error: %s", err)
		t.FailNow()
	}
	_, err = parser.Parse([]string{"--config", configFile})
	if err != nil {
		t.Logf("unexpected error parsing: %s", err)
		t.FailNow()
	}
	if cli.Name != "Brian" {
		t.Logf("expected name to be Brian, was %s", cli.Name)
		t.FailNow()
	}
}

func TestBasics2(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	configFile2 := filepath.Join(dir, "config.cue")
	os.WriteFile(configFile, []byte("name: Brian"), 0666)
	os.WriteFile(configFile2, []byte("hobby: \"sailing\""), 0666)

	var cli struct {
		Name   string         `name:"name" help:"a name" default:"Tom"`
		Hobby  string         `name:"hobby" help:"a hobby" default:"biking"`
		Config kongcue.Config `name:"config"`
	}

	parser, err := kong.New(&cli)
	if err != nil {
		t.Logf("unexpected error: %s", err)
		t.FailNow()
	}
	_, err = parser.Parse([]string{"--config", configFile, "--config", configFile2})
	if err != nil {
		t.Logf("unexpected error parsing: %s", err)
		t.FailNow()
	}
	if cli.Name != "Brian" {
		t.Logf("expected name to be Brian, was %s", cli.Name)
		t.FailNow()
	}
	if cli.Hobby != "sailing" {
		t.Logf("expected name to be sailing, was %s", cli.Hobby)
		t.FailNow()
	}
}

func TestNewResolver_GlobalFlag(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(`
verbose: 2
log_file: "/var/log/test.log"
`), 0644); err != nil {
		t.Fatal(err)
	}

	config, err := kongcue.LoadAndUnifyPaths([]string{configFile})
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	var cli testCLI
	parser, err := kong.New(&cli, kong.Resolvers(kongcue.NewResolver(config)))
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	_, err = parser.Parse([]string{"agent"})
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if cli.Verbose != 2 {
		t.Errorf("expected verbose 2, got %d", cli.Verbose)
	}

	if cli.LogFile != "/var/log/test.log" {
		t.Errorf("expected log_file '/var/log/test.log', got %q", cli.LogFile)
	}
}

func TestNewResolver_CommandFlag(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(`
agent:
  ca_url: "https://ca.example.com"
  auth: "my-auth-cmd"
`), 0644); err != nil {
		t.Fatal(err)
	}

	config, err := kongcue.LoadAndUnifyPaths([]string{configFile})
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	var cli testCLI
	parser, err := kong.New(&cli, kong.Resolvers(kongcue.NewResolver(config)))
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	_, err = parser.Parse([]string{"agent"})
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if cli.Agent.CaURL != "https://ca.example.com" {
		t.Errorf("expected ca_url 'https://ca.example.com', got %q", cli.Agent.CaURL)
	}

	if cli.Agent.Auth != "my-auth-cmd" {
		t.Errorf("expected auth 'my-auth-cmd', got %q", cli.Agent.Auth)
	}
}

func TestNewResolver_SliceFlag(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(`
agent:
  match:
    - "*.example.com"
    - "*.internal"
`), 0644); err != nil {
		t.Fatal(err)
	}

	config, err := kongcue.LoadAndUnifyPaths([]string{configFile})
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	var cli testCLI
	parser, err := kong.New(&cli, kong.Resolvers(kongcue.NewResolver(config)))
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	_, err = parser.Parse([]string{"agent"})
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(cli.Agent.Match) != 2 {
		t.Fatalf("expected 2 match patterns, got %d", len(cli.Agent.Match))
	}

	if cli.Agent.Match[0] != "*.example.com" {
		t.Errorf("expected first match '*.example.com', got %q", cli.Agent.Match[0])
	}

	if cli.Agent.Match[1] != "*.internal" {
		t.Errorf("expected second match '*.internal', got %q", cli.Agent.Match[1])
	}
}

func TestNewResolver_CLIOverridesConfig(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(`
agent:
  ca_url: "https://config-ca.example.com"
`), 0644); err != nil {
		t.Fatal(err)
	}

	config, err := kongcue.LoadAndUnifyPaths([]string{configFile})
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	var cli testCLI
	parser, err := kong.New(&cli, kong.Resolvers(kongcue.NewResolver(config)))
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	// CLI flag should override config
	_, err = parser.Parse([]string{"agent", "--ca-url", "https://cli-ca.example.com"})
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if cli.Agent.CaURL != "https://cli-ca.example.com" {
		t.Errorf("expected CLI override, got %q", cli.Agent.CaURL)
	}
}

func TestNewResolver_KebabToSnakeCase(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	// Config uses snake_case
	if err := os.WriteFile(configFile, []byte(`
log_file: "/var/log/test.log"
agent:
  ca_url: "https://ca.example.com"
`), 0644); err != nil {
		t.Fatal(err)
	}

	config, err := kongcue.LoadAndUnifyPaths([]string{configFile})
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	var cli testCLI
	parser, err := kong.New(&cli, kong.Resolvers(kongcue.NewResolver(config)))
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	_, err = parser.Parse([]string{"agent"})
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// Flags use kebab-case (--log-file, --ca-url), config uses snake_case
	if cli.LogFile != "/var/log/test.log" {
		t.Errorf("expected log_file mapping, got %q", cli.LogFile)
	}

	if cli.Agent.CaURL != "https://ca.example.com" {
		t.Errorf("expected ca_url mapping, got %q", cli.Agent.CaURL)
	}
}

func TestNewResolver_EmptyConfig(t *testing.T) {
	config, err := kongcue.LoadAndUnifyPaths([]string{"/nonexistent/path"})
	if err != nil {
		t.Fatalf("failed to load empty config: %v", err)
	}

	var cli testCLI
	parser, err := kong.New(&cli, kong.Resolvers(kongcue.NewResolver(config)))
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	// Should work with empty config - just uses defaults
	_, err = parser.Parse([]string{"agent", "--ca-url", "https://required.example.com"})
	if err != nil {
		t.Fatalf("failed to parse with empty config: %v", err)
	}
}
