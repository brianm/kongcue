package kongcue_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	kongcue "github.com/brianm/kongcue"
)

// Test CLI structure for schema tests
type schemaCLI struct {
	Verbose int      `short:"v" type:"counter"`
	LogFile string   `name:"log-file"`
	Debug   bool     `name:"debug"`
	Agent   agentCMD `cmd:"agent"`
}

type agentCMD struct {
	CaURL string   `name:"ca-url"`
	Match []string `name:"match"`
	Port  int      `name:"port" default:"8080"`
}

func TestGenerateSchema_BasicFields(t *testing.T) {
	var cli schemaCLI
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	config, _ := kongcue.LoadAndUnifyPaths([]string{})
	schema, err := kongcue.GenerateSchema(config.Context(), parser.Model, nil)
	if err != nil {
		t.Fatalf("failed to generate schema: %v", err)
	}

	// Verify schema exists
	if !schema.Exists() {
		t.Fatal("schema should exist")
	}

	// Verify schema accepts valid fields by unifying with a config
	validConfig := config.Context().CompileString(`{
		verbose: 2
		log_file: "/var/log/test.log"
		debug: true
	}`)
	unified := schema.Unify(validConfig)
	if err := unified.Err(); err != nil {
		t.Errorf("schema should accept valid fields: %v", err)
	}
}

func TestValidateConfig_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(`
verbose: 2
log_file: "/var/log/test.log"
agent:
  ca_url: "https://ca.example.com"
  match:
    - "*.example.com"
  port: 9090
`), 0644); err != nil {
		t.Fatal(err)
	}

	var cli schemaCLI
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	config, err := kongcue.LoadAndUnifyPaths([]string{configFile})
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	schema, err := kongcue.GenerateSchema(config.Context(), parser.Model, nil)
	if err != nil {
		t.Fatalf("failed to generate schema: %v", err)
	}

	// Should validate without error
	if err := kongcue.ValidateConfig(schema, config); err != nil {
		t.Errorf("valid config should not produce error: %v", err)
	}
}

func TestValidateConfig_UnknownField(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(`
verbose: 2
unknown_field: "should fail"
`), 0644); err != nil {
		t.Fatal(err)
	}

	var cli schemaCLI
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	config, err := kongcue.LoadAndUnifyPaths([]string{configFile})
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	schema, err := kongcue.GenerateSchema(config.Context(), parser.Model, nil)
	if err != nil {
		t.Fatalf("failed to generate schema: %v", err)
	}

	// Should fail with unknown field error
	err = kongcue.ValidateConfig(schema, config)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "unknown_field") {
		t.Errorf("error should mention unknown field, got: %v", err)
	}
}

func TestValidateConfig_UnknownNestedField(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(`
agent:
  ca_url: "https://ca.example.com"
  bad_field: "should fail"
`), 0644); err != nil {
		t.Fatal(err)
	}

	var cli schemaCLI
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	config, err := kongcue.LoadAndUnifyPaths([]string{configFile})
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	schema, err := kongcue.GenerateSchema(config.Context(), parser.Model, nil)
	if err != nil {
		t.Fatalf("failed to generate schema: %v", err)
	}

	// Should fail with unknown field error
	err = kongcue.ValidateConfig(schema, config)
	if err == nil {
		t.Fatal("expected error for unknown nested field")
	}
	if !strings.Contains(err.Error(), "bad_field") {
		t.Errorf("error should mention bad_field, got: %v", err)
	}
}

func TestValidateConfig_TypeMismatch(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	// Use string where int is expected
	if err := os.WriteFile(configFile, []byte(`
verbose: "not-a-number"
`), 0644); err != nil {
		t.Fatal(err)
	}

	var cli schemaCLI
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	config, err := kongcue.LoadAndUnifyPaths([]string{configFile})
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	schema, err := kongcue.GenerateSchema(config.Context(), parser.Model, nil)
	if err != nil {
		t.Fatalf("failed to generate schema: %v", err)
	}

	// Should fail validation due to type mismatch
	err = kongcue.ValidateConfig(schema, config)
	if err == nil {
		t.Fatal("expected error for type mismatch")
	}
}

func TestAllowUnknownFields_Option(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(`
name: "Test"
extra_field: "should be allowed"
`), 0644); err != nil {
		t.Fatal(err)
	}

	var cli struct {
		Name   string         `name:"name" default:"default"`
		Config kongcue.Config `name:"config"`
	}

	// With AllowUnknownFields(), unknown fields should be accepted
	parser, err := kong.New(&cli, kongcue.AllowUnknownFields())
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	_, err = parser.Parse([]string{"--config", configFile})
	if err != nil {
		t.Errorf("unknown fields should be allowed with AllowUnknownFields(): %v", err)
	}
	if cli.Name != "Test" {
		t.Errorf("expected name 'Test', got %q", cli.Name)
	}
}

func TestAllowUnknownFields_PathSpecific(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(`
name: "Test"
agent:
  ca_url: "https://example.com"
  extra_agent_field: "should be allowed"
`), 0644); err != nil {
		t.Fatal(err)
	}

	var cli struct {
		Name   string `name:"name" default:"default"`
		Agent  struct {
			CaURL string `name:"ca-url"`
		} `cmd:"agent"`
		Config kongcue.Config `name:"config"`
	}

	// AllowUnknownFields("agent") should allow unknown fields in agent section
	parser, err := kong.New(&cli, kongcue.AllowUnknownFields("agent"))
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	_, err = parser.Parse([]string{"--config", configFile, "agent"})
	if err != nil {
		t.Errorf("unknown fields in 'agent' should be allowed: %v", err)
	}
	if cli.Name != "Test" {
		t.Errorf("expected name 'Test', got %q", cli.Name)
	}
}

func TestAllowUnknownFields_PathSpecific_RejectsElsewhere(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(`
name: "Test"
unknown_root_field: "should NOT be allowed"
agent:
  ca_url: "https://example.com"
`), 0644); err != nil {
		t.Fatal(err)
	}

	var cli struct {
		Name   string `name:"name" default:"default"`
		Agent  struct {
			CaURL string `name:"ca-url"`
		} `cmd:"agent"`
		Config kongcue.Config `name:"config"`
	}

	// AllowUnknownFields("agent") should still reject unknown fields at root
	parser, err := kong.New(&cli, kongcue.AllowUnknownFields("agent"))
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	_, err = parser.Parse([]string{"--config", configFile, "agent"})
	if err == nil {
		t.Error("expected error for unknown field at root when only 'agent' path allows unknown")
	}
}

func TestAllowUnknownFields_MultiplePaths(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(`
name: "Test"
agent:
  ca_url: "https://example.com"
  extra_agent_field: "allowed"
server:
  extra_server_field: "also allowed"
`), 0644); err != nil {
		t.Fatal(err)
	}

	var cli struct {
		Name   string `name:"name" default:"default"`
		Agent  struct {
			CaURL string `name:"ca-url"`
		} `cmd:"agent"`
		Server struct {
			Port int `name:"port"`
		} `cmd:"server"`
		Config kongcue.Config `name:"config"`
	}

	// AllowUnknownFields with multiple paths
	parser, err := kong.New(&cli, kongcue.AllowUnknownFields("agent", "server"))
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	_, err = parser.Parse([]string{"--config", configFile, "agent"})
	if err != nil {
		t.Errorf("unknown fields in 'agent' and 'server' should be allowed: %v", err)
	}
}

func TestAllowUnknownFields_NonCommandPath(t *testing.T) {
	// Test that AllowUnknownFields works for paths that aren't commands
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(`
name: "Test"
extra:
  anything: "goes here"
  nested:
    deep: 123
`), 0644); err != nil {
		t.Fatal(err)
	}

	var cli struct {
		Name   string         `name:"name" default:"default"`
		Config kongcue.Config `name:"config"`
	}

	// AllowUnknownFields("extra") should allow the "extra" section even though
	// it's not a command - it just doesn't exist in the CLI struct
	parser, err := kong.New(&cli, kongcue.AllowUnknownFields("extra"))
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	_, err = parser.Parse([]string{"--config", configFile})
	if err != nil {
		t.Errorf("'extra' path should be allowed with AllowUnknownFields(\"extra\"): %v", err)
	}
	if cli.Name != "Test" {
		t.Errorf("expected name 'Test', got %q", cli.Name)
	}
}

func TestBeforeResolve_RejectsUnknownField(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(`
name: "Brian"
typo_field: "should fail"
`), 0644); err != nil {
		t.Fatal(err)
	}

	var cli struct {
		Name   string         `name:"name" default:"default"`
		Config kongcue.Config `name:"config"`
	}

	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	_, err = parser.Parse([]string{"--config", configFile})
	if err == nil {
		t.Fatal("expected error for unknown field in config")
	}
	if !strings.Contains(err.Error(), "typo_field") {
		t.Errorf("error should mention typo_field, got: %v", err)
	}
}

func TestBeforeResolve_AcceptsValidConfig(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(`name: "Brian"`), 0644); err != nil {
		t.Fatal(err)
	}

	var cli struct {
		Name   string         `name:"name" default:"default"`
		Config kongcue.Config `name:"config"`
	}

	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	_, err = parser.Parse([]string{"--config", configFile})
	if err != nil {
		t.Fatalf("valid config should not produce error: %v", err)
	}
	if cli.Name != "Brian" {
		t.Errorf("expected name 'Brian', got %q", cli.Name)
	}
}
