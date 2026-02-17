// Where: services/agent/internal/cni/generator_test.go
// What: Tests for CNI config generation.
// Why: Ensure subnet and DNS settings are reflected in generated configs.
package cni

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	if err := GenerateConfig(dir, "", testCNIName, testCNIBridge); err != nil {
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
	if err := GenerateConfig(dir, "", testCNIName, testCNIBridge); err != nil {
		t.Fatalf("GenerateConfig (default): %v", err)
	}
	cfg = readConfig(t, dir)
	plugin = cfg.Plugins[0]
	if plugin.DNS.Nameservers[0] != "10.88.0.1" {
		t.Fatalf("dns default mismatch: %s", plugin.DNS.Nameservers[0])
	}
}
