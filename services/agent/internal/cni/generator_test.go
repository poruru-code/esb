// Where: services/agent/internal/cni/generator_test.go
// What: Tests for CNI config generation.
// Why: Ensure subnet and DNS settings are reflected in generated configs.
package cni

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testCNIName   = "esb-net"
	testCNIBridge = "esb0"
)

func readConfig(t *testing.T, dir string) Config {
	t.Helper()
	path := filepath.Join(dir, fmt.Sprintf("10-%s.conflist", testCNIName))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	return cfg
}

func TestGenerateConfig_CustomSubnetAndDNS(t *testing.T) {
	t.Setenv("CNI_DNS_SERVER", "10.99.0.53")
	dir := t.TempDir()

	if err := GenerateConfig(dir, "10.20.0.0/24", testCNIName, testCNIBridge); err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	cfg := readConfig(t, dir)
	if len(cfg.Plugins) == 0 {
		t.Fatal("expected at least one plugin")
	}
	plugin := cfg.Plugins[0]
	if plugin.IPAM == nil {
		t.Fatal("expected IPAM config")
	}
	if plugin.IPAM.Subnet != "10.20.0.0/24" {
		t.Fatalf("subnet mismatch: %s", plugin.IPAM.Subnet)
	}
	if plugin.IPAM.RangeStart != "10.20.0.1" {
		t.Fatalf("range start mismatch: %s", plugin.IPAM.RangeStart)
	}
	if plugin.IPAM.RangeEnd != "10.20.0.254" {
		t.Fatalf("range end mismatch: %s", plugin.IPAM.RangeEnd)
	}
	if plugin.DNS == nil || len(plugin.DNS.Nameservers) == 0 {
		t.Fatal("expected DNS nameserver")
	}
	if plugin.DNS.Nameservers[0] != "10.99.0.53" {
		t.Fatalf("dns mismatch: %s", plugin.DNS.Nameservers[0])
	}
}

func TestGenerateConfig_DNSServerFallbacks(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("CNI_DNS_SERVER", "")
	t.Setenv("CNI_GW_IP", "10.88.0.2")
	if err := GenerateConfig(dir, "10.88.0.0/16", testCNIName, testCNIBridge); err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}
	cfg := readConfig(t, dir)
	plugin := cfg.Plugins[0]
	if plugin.DNS == nil || len(plugin.DNS.Nameservers) == 0 {
		t.Fatal("expected DNS nameserver")
	}
	if plugin.DNS.Nameservers[0] != "10.88.0.2" {
		t.Fatalf("dns fallback mismatch: %s", plugin.DNS.Nameservers[0])
	}

	t.Setenv("CNI_GW_IP", "")
	if err := GenerateConfig(dir, "10.88.0.0/16", testCNIName, testCNIBridge); err != nil {
		t.Fatalf("GenerateConfig (default): %v", err)
	}
	cfg = readConfig(t, dir)
	plugin = cfg.Plugins[0]
	if plugin.DNS.Nameservers[0] != "10.88.0.1" {
		t.Fatalf("dns default mismatch: %s", plugin.DNS.Nameservers[0])
	}
}

func TestGenerateConfig_RequiresResolvedIdentityInputs(t *testing.T) {
	dir := t.TempDir()
	if err := GenerateConfig(dir, "", "", ""); err == nil {
		t.Fatal("expected error when identity inputs are empty")
	}
}

func TestGenerateConfig_DNSServerFallsBackToSubnetGateway(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CNI_DNS_SERVER", "")
	t.Setenv("CNI_GW_IP", "")

	if err := GenerateConfig(dir, "10.44.16.0/20", testCNIName, testCNIBridge); err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	cfg := readConfig(t, dir)
	plugin := cfg.Plugins[0]
	if plugin.DNS == nil || len(plugin.DNS.Nameservers) == 0 {
		t.Fatal("expected DNS nameserver")
	}
	if plugin.DNS.Nameservers[0] != "10.44.16.1" {
		t.Fatalf("dns subnet fallback mismatch: %s", plugin.DNS.Nameservers[0])
	}
}

func TestWriteIdentityFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity.env")

	if err := WriteIdentityFile(path, "acme-net", "esb-acme123456", "10.44.16.0/20"); err != nil {
		t.Fatalf("WriteIdentityFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read identity file: %v", err)
	}

	text := string(data)
	for _, expect := range []string{
		"CNI_NETWORK='acme-net'",
		"CNI_BRIDGE='esb-acme123456'",
		"CNI_SUBNET='10.44.16.0/20'",
		"CNI_GW_IP='10.44.16.1'",
	} {
		if !strings.Contains(text, expect+"\n") {
			t.Fatalf("identity file missing %q in %q", expect, text)
		}
	}
}

func TestWriteIdentityFile_ShellEscapesValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity.env")
	sentinel := filepath.Join(dir, "pwned")
	malicious := fmt.Sprintf("$(touch %s)", sentinel)

	if err := WriteIdentityFile(path, malicious, "esb-acme123456", "10.44.16.0/20"); err != nil {
		t.Fatalf("WriteIdentityFile: %v", err)
	}

	cmd := exec.Command(
		"sh",
		"-c",
		`. "$1"; printf '%s' "$CNI_NETWORK"`,
		"sh",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("source identity file: %v", err)
	}
	if got := string(out); got != malicious {
		t.Fatalf("CNI_NETWORK mismatch: got %q want %q", got, malicious)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("malicious command was executed; sentinel exists: %v", err)
	}
}

func TestCollectSubnetClaims(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"10-self.conflist": `{
  "name": "self-net",
  "plugins": [{"type":"bridge","ipam":{"type":"host-local","subnet":"10.10.0.0/24"}}]
}`,
		"20-other.conflist": `{
  "name": "other-net",
  "plugins": [{"type":"bridge","ipam":{"type":"host-local","subnet":"10.20.0.0/24"}}]
}`,
		"30-inline.conf": `{
  "name": "inline-net",
  "ipam": {"type":"host-local","subnet":"10.30.0.0/24"}
}`,
		"40-invalid.conflist": "{invalid json",
	}
	for name, payload := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	claims, err := CollectSubnetClaims(dir, "self-net")
	if err != nil {
		t.Fatalf("CollectSubnetClaims: %v", err)
	}

	if _, ok := claims["10.10.0.0/24"]; ok {
		t.Fatalf("self network subnet should be excluded: %#v", claims)
	}
	if got := claims["10.20.0.0/24"]; got != "other-net" {
		t.Fatalf("claim mismatch for 10.20.0.0/24: %q", got)
	}
	if got := claims["10.30.0.0/24"]; got != "inline-net" {
		t.Fatalf("claim mismatch for 10.30.0.0/24: %q", got)
	}
}

func TestCollectSubnetClaimsMissingDir(t *testing.T) {
	claims, err := CollectSubnetClaims(filepath.Join(t.TempDir(), "missing"), "self-net")
	if err != nil {
		t.Fatalf("CollectSubnetClaims missing dir: %v", err)
	}
	if len(claims) != 0 {
		t.Fatalf("expected no claims, got %#v", claims)
	}
}
