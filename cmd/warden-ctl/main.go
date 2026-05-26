// Command warden-ctl is a config-as-code CLI for managing Warden resources.
// It supports three subcommands:
//
//	apply  — reconcile desired state described in a YAML file against the server
//	export — fetch current server state and print as YAML or JSON
//	diff   — compute the difference between file state and server state
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Config schema types
// ---------------------------------------------------------------------------

// WardenConfig is the top-level structure for a warden-config.yaml file.
type WardenConfig struct {
	PBACPolicies  []PBACPolicy   `yaml:"pbac_policies"  json:"pbac_policies"`
	HoldTemplates []HoldTemplate `yaml:"hold_templates"  json:"hold_templates"`
	VIPIdentities []VIPIdentity  `yaml:"vip_identities"  json:"vip_identities"`
	Instances     []Instance     `yaml:"instances"       json:"instances"`
}

// PBACPolicy describes a policy-based access control policy.
type PBACPolicy struct {
	Name    string                 `yaml:"name"    json:"name"`
	Enabled bool                   `yaml:"enabled" json:"enabled"`
	Config  map[string]interface{} `yaml:"config"  json:"config"`
}

// HoldTemplate describes a reusable legal hold template.
type HoldTemplate struct {
	Name           string `yaml:"name"            json:"name"`
	Description    string `yaml:"description"     json:"description"`
	ProviderGlob   string `yaml:"provider_glob"   json:"provider_glob"`
	ExpirationDays int    `yaml:"expiration_days" json:"expiration_days"`
}

// VIPIdentity marks a user as requiring elevated protection.
type VIPIdentity struct {
	Email  string `yaml:"email"  json:"email"`
	Reason string `yaml:"reason" json:"reason"`
}

// Instance describes an integration instance.
type Instance struct {
	Name        string                 `yaml:"name"        json:"name"`
	PluginID    string                 `yaml:"plugin_id"   json:"plugin_id"`
	Credentials map[string]interface{} `yaml:"credentials" json:"credentials"`
}

// ---------------------------------------------------------------------------
// HTTP client helpers
// ---------------------------------------------------------------------------

type client struct {
	serverURL string
	token     string
	http      *http.Client
}

func newClient(serverURL, token string) *client {
	return &client{
		serverURL: strings.TrimRight(serverURL, "/"),
		token:     token,
		http:      &http.Client{},
	}
}

func (c *client) get(path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, c.serverURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func (c *client) post(path string, payload interface{}) error {
	return c.sendJSON(http.MethodPost, path, payload)
}

func (c *client) put(path string, payload interface{}) error {
	return c.sendJSON(http.MethodPut, path, payload)
}

func (c *client) sendJSON(method, path string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(method, c.serverURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Server state fetching
// ---------------------------------------------------------------------------

// ServerState holds the current state fetched from the Warden server.
type ServerState struct {
	PBACPolicies  []PBACPolicy   `json:"pbac_policies"`
	HoldTemplates []HoldTemplate `json:"hold_templates"`
	VIPIdentities []VIPIdentity  `json:"vip_identities"`
	Instances     []Instance     `json:"instances"`
}

func fetchServerState(c *client) (*WardenConfig, error) {
	state := &WardenConfig{}

	// Fetch PBAC policies
	data, err := c.get("/api/v1/internal/admin/pbac-policies")
	if err != nil {
		return nil, fmt.Errorf("fetch pbac policies: %w", err)
	}
	if err := json.Unmarshal(data, &state.PBACPolicies); err != nil {
		// Server may wrap in an object; try unwrapping
		var wrapper struct {
			Policies []PBACPolicy `json:"policies"`
		}
		if jerr := json.Unmarshal(data, &wrapper); jerr == nil {
			state.PBACPolicies = wrapper.Policies
		}
	}

	// Fetch hold templates
	data, err = c.get("/api/v1/internal/admin/hold-templates")
	if err != nil {
		return nil, fmt.Errorf("fetch hold templates: %w", err)
	}
	if err := json.Unmarshal(data, &state.HoldTemplates); err != nil {
		var wrapper struct {
			Templates []HoldTemplate `json:"templates"`
		}
		if jerr := json.Unmarshal(data, &wrapper); jerr == nil {
			state.HoldTemplates = wrapper.Templates
		}
	}

	// Fetch VIP identities
	data, err = c.get("/api/v1/internal/admin/vip-identities")
	if err != nil {
		return nil, fmt.Errorf("fetch vip identities: %w", err)
	}
	if err := json.Unmarshal(data, &state.VIPIdentities); err != nil {
		var wrapper struct {
			Identities []VIPIdentity `json:"identities"`
		}
		if jerr := json.Unmarshal(data, &wrapper); jerr == nil {
			state.VIPIdentities = wrapper.Identities
		}
	}

	// Fetch instances
	data, err = c.get("/api/v1/internal/admin/instances")
	if err != nil {
		return nil, fmt.Errorf("fetch instances: %w", err)
	}
	if err := json.Unmarshal(data, &state.Instances); err != nil {
		var wrapper struct {
			Instances []Instance `json:"instances"`
		}
		if jerr := json.Unmarshal(data, &wrapper); jerr == nil {
			state.Instances = wrapper.Instances
		}
	}

	return state, nil
}

// ---------------------------------------------------------------------------
// Diff helpers
// ---------------------------------------------------------------------------

// diffLine represents a single line in a colored unified diff.
type diffLine struct {
	prefix string // "+", "-", " "
	text   string
}

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
)

func marshalYAML(v interface{}) string {
	b, err := yaml.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// printDiff produces a simplified colored diff between desired and actual YAML.
func printDiff(section, desired, actual string) {
	if desired == actual {
		return
	}
	fmt.Printf("%s--- server/%s%s\n", colorRed, section, colorReset)
	fmt.Printf("%s+++ file/%s%s\n", colorGreen, section, colorReset)

	aLines := strings.Split(actual, "\n")
	dLines := strings.Split(desired, "\n")

	// Simple line-by-line diff (not LCS — sufficient for config reconciliation)
	aSet := make(map[string]bool, len(aLines))
	dSet := make(map[string]bool, len(dLines))
	for _, l := range aLines {
		aSet[l] = true
	}
	for _, l := range dLines {
		dSet[l] = true
	}

	for _, l := range aLines {
		if !dSet[l] {
			fmt.Printf("%s- %s%s\n", colorRed, l, colorReset)
		}
	}
	for _, l := range dLines {
		if !aSet[l] {
			fmt.Printf("%s+ %s%s\n", colorGreen, l, colorReset)
		}
	}
}

// ---------------------------------------------------------------------------
// Reconciliation helpers
// ---------------------------------------------------------------------------

func applyPBACPolicies(c *client, desired []PBACPolicy, actual []PBACPolicy) error {
	actualMap := make(map[string]PBACPolicy, len(actual))
	for _, p := range actual {
		actualMap[p.Name] = p
	}

	for _, p := range desired {
		if _, exists := actualMap[p.Name]; exists {
			if err := c.put("/api/v1/internal/admin/pbac-policies/"+p.Name, p); err != nil {
				return fmt.Errorf("update policy %q: %w", p.Name, err)
			}
			fmt.Printf("  updated pbac_policy %q\n", p.Name)
		} else {
			if err := c.post("/api/v1/internal/admin/pbac-policies", p); err != nil {
				return fmt.Errorf("create policy %q: %w", p.Name, err)
			}
			fmt.Printf("  created pbac_policy %q\n", p.Name)
		}
	}
	return nil
}

func applyHoldTemplates(c *client, desired []HoldTemplate, actual []HoldTemplate) error {
	actualMap := make(map[string]HoldTemplate, len(actual))
	for _, t := range actual {
		actualMap[t.Name] = t
	}

	for _, t := range desired {
		if _, exists := actualMap[t.Name]; exists {
			if err := c.put("/api/v1/internal/admin/hold-templates/"+t.Name, t); err != nil {
				return fmt.Errorf("update template %q: %w", t.Name, err)
			}
			fmt.Printf("  updated hold_template %q\n", t.Name)
		} else {
			if err := c.post("/api/v1/internal/admin/hold-templates", t); err != nil {
				return fmt.Errorf("create template %q: %w", t.Name, err)
			}
			fmt.Printf("  created hold_template %q\n", t.Name)
		}
	}
	return nil
}

func applyVIPIdentities(c *client, desired []VIPIdentity, actual []VIPIdentity) error {
	actualMap := make(map[string]VIPIdentity, len(actual))
	for _, v := range actual {
		actualMap[v.Email] = v
	}

	for _, v := range desired {
		if _, exists := actualMap[v.Email]; exists {
			if err := c.put("/api/v1/internal/admin/vip-identities/"+v.Email, v); err != nil {
				return fmt.Errorf("update vip identity %q: %w", v.Email, err)
			}
			fmt.Printf("  updated vip_identity %q\n", v.Email)
		} else {
			if err := c.post("/api/v1/internal/admin/vip-identities", v); err != nil {
				return fmt.Errorf("create vip identity %q: %w", v.Email, err)
			}
			fmt.Printf("  created vip_identity %q\n", v.Email)
		}
	}
	return nil
}

func applyInstances(c *client, desired []Instance, actual []Instance) error {
	actualMap := make(map[string]Instance, len(actual))
	for _, i := range actual {
		actualMap[i.Name] = i
	}

	for _, inst := range desired {
		// Expand {{ env.VAR }} template expressions in credentials
		expanded := expandEnvCredentials(inst.Credentials)
		inst.Credentials = expanded

		if _, exists := actualMap[inst.Name]; exists {
			if err := c.put("/api/v1/internal/admin/instances/"+inst.Name, inst); err != nil {
				return fmt.Errorf("update instance %q: %w", inst.Name, err)
			}
			fmt.Printf("  updated instance %q\n", inst.Name)
		} else {
			if err := c.post("/api/v1/internal/admin/instances", inst); err != nil {
				return fmt.Errorf("create instance %q: %w", inst.Name, err)
			}
			fmt.Printf("  created instance %q\n", inst.Name)
		}
	}
	return nil
}

// expandEnvCredentials replaces "{{ env.VAR }}" patterns with the value of
// the corresponding environment variable.
func expandEnvCredentials(creds map[string]interface{}) map[string]interface{} {
	if creds == nil {
		return nil
	}
	out := make(map[string]interface{}, len(creds))
	for k, v := range creds {
		if s, ok := v.(string); ok {
			s = strings.TrimSpace(s)
			if strings.HasPrefix(s, "{{") && strings.HasSuffix(s, "}}") {
				inner := strings.TrimSpace(s[2 : len(s)-2])
				if strings.HasPrefix(inner, "env.") {
					varName := inner[4:]
					s = os.Getenv(varName)
				}
			}
			out[k] = s
		} else {
			out[k] = v
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Subcommands
// ---------------------------------------------------------------------------

func cmdApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	file := fs.String("file", "", "path to warden-config.yaml (required)")
	server := fs.String("server", "", "Warden server URL, e.g. https://warden.example.com (required)")
	token := fs.String("token", "", "API token (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" || *server == "" || *token == "" {
		fs.Usage()
		return fmt.Errorf("--file, --server, and --token are required")
	}

	desired, err := loadConfig(*file)
	if err != nil {
		return err
	}

	c := newClient(*server, *token)
	actual, err := fetchServerState(c)
	if err != nil {
		return fmt.Errorf("fetch server state: %w", err)
	}

	fmt.Println("Applying configuration...")

	if err := applyPBACPolicies(c, desired.PBACPolicies, actual.PBACPolicies); err != nil {
		return err
	}
	if err := applyHoldTemplates(c, desired.HoldTemplates, actual.HoldTemplates); err != nil {
		return err
	}
	if err := applyVIPIdentities(c, desired.VIPIdentities, actual.VIPIdentities); err != nil {
		return err
	}
	if err := applyInstances(c, desired.Instances, actual.Instances); err != nil {
		return err
	}

	fmt.Println("Done.")
	return nil
}

func cmdExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	server := fs.String("server", "", "Warden server URL (required)")
	token := fs.String("token", "", "API token (required)")
	format := fs.String("format", "yaml", "output format: yaml or json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *server == "" || *token == "" {
		fs.Usage()
		return fmt.Errorf("--server and --token are required")
	}

	c := newClient(*server, *token)
	state, err := fetchServerState(c)
	if err != nil {
		return err
	}

	switch *format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(state)
	case "yaml":
		return yaml.NewEncoder(os.Stdout).Encode(state)
	default:
		return fmt.Errorf("unknown format %q; use yaml or json", *format)
	}
}

func cmdDiff(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	file := fs.String("file", "", "path to warden-config.yaml (required)")
	server := fs.String("server", "", "Warden server URL (required)")
	token := fs.String("token", "", "API token (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" || *server == "" || *token == "" {
		fs.Usage()
		return fmt.Errorf("--file, --server, and --token are required")
	}

	desired, err := loadConfig(*file)
	if err != nil {
		return err
	}

	c := newClient(*server, *token)
	actual, err := fetchServerState(c)
	if err != nil {
		return fmt.Errorf("fetch server state: %w", err)
	}

	printDiff("pbac_policies", marshalYAML(desired.PBACPolicies), marshalYAML(actual.PBACPolicies))
	printDiff("hold_templates", marshalYAML(desired.HoldTemplates), marshalYAML(actual.HoldTemplates))
	printDiff("vip_identities", marshalYAML(desired.VIPIdentities), marshalYAML(actual.VIPIdentities))
	printDiff("instances", marshalYAML(desired.Instances), marshalYAML(actual.Instances))

	return nil
}

// ---------------------------------------------------------------------------
// Config file loading
// ---------------------------------------------------------------------------

func loadConfig(path string) (*WardenConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config file: %w", err)
	}
	defer f.Close()

	var cfg WardenConfig
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}
	return &cfg, nil
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: warden-ctl <command> [flags]\n\nCommands:\n  apply   Reconcile desired state from a config file\n  export  Export current server state\n  diff    Show differences between file and server state\n")
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "apply":
		err = cmdApply(os.Args[2:])
	case "export":
		err = cmdExport(os.Args[2:])
	case "diff":
		err = cmdDiff(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
