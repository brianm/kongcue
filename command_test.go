package kongcue

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

type configDocCLI struct {
	Verbose   int            `short:"v" type:"counter"`
	LogFile   string         `name:"log-file"`
	Agent     configDocAgent `cmd:""`
	ConfigDoc ConfigDoc      `cmd:"config-doc"`
}

type configDocAgent struct {
	CaURL string   `name:"ca-url"`
	Match []string `name:"match"`
	Port  int      `name:"port" default:"8080"`
}

func TestConfigDoc_GeneratesSchema(t *testing.T) {
	var cli configDocCLI
	var buf bytes.Buffer
	cli.ConfigDoc.Output = &buf

	parser, err := kong.New(&cli, Options())
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	_, err = parser.Parse([]string{"config-doc"})
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	output := buf.String()

	// Check for #Root definition
	if !strings.Contains(output, "#Root:") {
		t.Errorf("expected #Root definition, got:\n%s", output)
	}

	// Check for #Agent definition
	if !strings.Contains(output, "#Agent:") {
		t.Errorf("expected #Agent definition, got:\n%s", output)
	}

	// Check for close() wrapping (default behavior)
	if !strings.Contains(output, "close(") {
		t.Errorf("expected close() wrapping, got:\n%s", output)
	}

	// Check for snake_case field names
	if !strings.Contains(output, "log_file?:") {
		t.Errorf("expected log_file field, got:\n%s", output)
	}
	if !strings.Contains(output, "ca_url?:") {
		t.Errorf("expected ca_url field in Agent, got:\n%s", output)
	}

	// Check that agent references #Agent (allow whitespace)
	if !strings.Contains(output, "agent?:") || !strings.Contains(output, "#Agent") {
		t.Errorf("expected agent?: #Agent reference, got:\n%s", output)
	}

	// ConfigDoc should not appear in schema
	if strings.Contains(output, "ConfigDoc") || strings.Contains(output, "config-doc") {
		t.Errorf("ConfigDoc should not appear in schema, got:\n%s", output)
	}
}

func TestConfigDoc_WithAllowUnknownFields(t *testing.T) {
	var cli configDocCLI
	var buf bytes.Buffer
	cli.ConfigDoc.Output = &buf

	parser, err := kong.New(&cli, AllowUnknownFields("agent"))
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	_, err = parser.Parse([]string{"config-doc"})
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	output := buf.String()

	// Root should still be closed
	if !strings.Contains(output, "#Root: close(") {
		t.Errorf("expected #Root to be closed, got:\n%s", output)
	}

	// Agent should NOT be wrapped in close() since it allows unknown fields
	// It should just be #Agent: { ... } without close()
	if strings.Contains(output, "#Agent: close(") {
		t.Errorf("expected #Agent to NOT be closed (unknown fields allowed), got:\n%s", output)
	}
}

func TestConfigDoc_WithAllowUnknownFieldsCustomPath(t *testing.T) {
	var cli configDocCLI
	var buf bytes.Buffer
	cli.ConfigDoc.Output = &buf

	parser, err := kong.New(&cli, AllowUnknownFields("custom"))
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	_, err = parser.Parse([]string{"config-doc"})
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	output := buf.String()

	// Should have custom?: _ field in root (allow whitespace)
	if !strings.Contains(output, "custom?:") || !strings.Contains(output, "_") {
		t.Errorf("expected custom?: _ field for allowed path, got:\n%s", output)
	}
}

func TestConfigDoc_WithAllowAllUnknownFields(t *testing.T) {
	var cli configDocCLI
	var buf bytes.Buffer
	cli.ConfigDoc.Output = &buf

	parser, err := kong.New(&cli, AllowUnknownFields())
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	_, err = parser.Parse([]string{"config-doc"})
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	output := buf.String()

	// Nothing should be wrapped in close() when all unknown fields are allowed
	if strings.Contains(output, "close(") {
		t.Errorf("expected no close() when all unknown fields allowed, got:\n%s", output)
	}
}

type nestedCLI struct {
	Server    nestedServer `cmd:""`
	ConfigDoc ConfigDoc    `cmd:"config-doc"`
}

type nestedServer struct {
	Port int       `name:"port"`
	TLS  nestedTLS `cmd:""`
}

type nestedTLS struct {
	CertFile string `name:"cert-file"`
}

func TestConfigDoc_NestedCommands(t *testing.T) {
	var cli nestedCLI
	var buf bytes.Buffer
	cli.ConfigDoc.Output = &buf

	parser, err := kong.New(&cli, Options())
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	_, err = parser.Parse([]string{"config-doc"})
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	output := buf.String()

	// Check for nested definition naming
	if !strings.Contains(output, "#Server:") {
		t.Errorf("expected #Server definition, got:\n%s", output)
	}
	if !strings.Contains(output, "#ServerTls:") {
		t.Errorf("expected #ServerTls definition for nested command, got:\n%s", output)
	}

	// Check that Server references ServerTls (allow whitespace)
	if !strings.Contains(output, "tls?:") || !strings.Contains(output, "#ServerTls") {
		t.Errorf("expected tls?: #ServerTls reference in Server, got:\n%s", output)
	}

	// ConfigDoc should not appear in schema
	if strings.Contains(output, "ConfigDoc") || strings.Contains(output, "config-doc") {
		t.Errorf("ConfigDoc should not appear in schema, got:\n%s", output)
	}
}

func TestConfigDoc_RequiredFields(t *testing.T) {
	// Use a command with required fields to test schema generation
	var cli struct {
		Server struct {
			Host string `name:"host" required:""`
			Port int    `name:"port"`
		} `cmd:"server"`
		ConfigDoc ConfigDoc `cmd:"config-doc"`
	}
	var buf bytes.Buffer
	cli.ConfigDoc.Output = &buf

	parser, err := kong.New(&cli, Options())
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	_, err = parser.Parse([]string{"config-doc"})
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	output := buf.String()

	// Required field should NOT have ? (optional marker)
	// It should appear as "host: string" not "host?: string"
	if strings.Contains(output, "host?:") {
		t.Errorf("required field 'host' should not be optional, got:\n%s", output)
	}
	if !strings.Contains(output, "host:") {
		t.Errorf("expected 'host:' field in schema, got:\n%s", output)
	}

	// Optional field SHOULD have ?
	if !strings.Contains(output, "port?:") {
		t.Errorf("optional field 'port' should have '?', got:\n%s", output)
	}
}

func TestConfigDoc_HelpText(t *testing.T) {
	var cli struct {
		Name   string `name:"name" help:"The user's name"`
		Age    int    `name:"age" help:"Age in years"`
		NoHelp string `name:"no-help"`
		Server struct {
			Host string `name:"host" help:"Server hostname"`
		} `cmd:"server" help:"Server configuration"`
		ConfigDoc ConfigDoc `cmd:"config-doc"`
	}
	var buf bytes.Buffer
	cli.ConfigDoc.Output = &buf

	parser, err := kong.New(&cli, Options())
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	_, err = parser.Parse([]string{"config-doc"})
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	output := buf.String()

	// Check help text appears as comments for flags
	if !strings.Contains(output, "// The user's name") {
		t.Errorf("expected help text comment for 'name', got:\n%s", output)
	}
	if !strings.Contains(output, "// Age in years") {
		t.Errorf("expected help text comment for 'age', got:\n%s", output)
	}

	// Check help text appears for command references
	if !strings.Contains(output, "// Server configuration") {
		t.Errorf("expected help text comment for 'server' command, got:\n%s", output)
	}

	// Check help text appears for flags in command definitions
	if !strings.Contains(output, "// Server hostname") {
		t.Errorf("expected help text comment for 'host' in Server, got:\n%s", output)
	}
}

func TestPascalCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"agent", "Agent"},
		{"ca-url", "CaUrl"},
		{"my_cmd", "MyCmd"},
		{"server-tls", "ServerTls"},
		{"a", "A"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := pascalCase(tt.input)
			if result != tt.expected {
				t.Errorf("pascalCase(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCommandDefName(t *testing.T) {
	tests := []struct {
		path     []string
		expected string
	}{
		{[]string{"agent"}, "Agent"},
		{[]string{"server", "tls"}, "ServerTls"},
		{[]string{"foo", "bar", "baz"}, "FooBarBaz"},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.path, "."), func(t *testing.T) {
			result := commandDefName(tt.path)
			if result != tt.expected {
				t.Errorf("commandDefName(%v) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}
