package identity

import (
	"net"
	"strings"
	"testing"
)

func TestResolveStackIdentityFrom(t *testing.T) {
	tests := []struct {
		name              string
		brandSlug         string
		projectName       string
		envName           string
		containersNetwork string
		wantBrand         string
		wantSource        string
		wantErr           bool
	}{
		{
			name:       "explicit brand slug wins",
			brandSlug:  "Acme_Main",
			wantBrand:  "acme-main",
			wantSource: EnvBrandSlug,
		},
		{
			name:        "project and env derive brand",
			projectName: "acme-dev",
			envName:     "dev",
			wantBrand:   "acme",
			wantSource:  EnvProjectName,
		},
		{
			name:              "network fallback strips external and env",
			containersNetwork: "acme-prod-external",
			envName:           "prod",
			wantBrand:         "acme",
			wantSource:        EnvContainersNetwork,
		},
		{
			name:    "missing identity inputs fails",
			wantErr: true,
		},
		{
			name:              "default bridge network is ignored and fails",
			containersNetwork: "bridge",
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveStackIdentityFrom(tt.brandSlug, tt.projectName, tt.envName, tt.containersNetwork)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.BrandSlug != tt.wantBrand {
				t.Fatalf("brand = %q, want %q", got.BrandSlug, tt.wantBrand)
			}
			if got.Source != tt.wantSource {
				t.Fatalf("source = %q, want %q", got.Source, tt.wantSource)
			}
		})
	}
}

func TestStackIdentityDerivedValues(t *testing.T) {
	id := StackIdentity{BrandSlug: "acme-core"}
	if got := id.RuntimeNamespace(); got != "acme-core" {
		t.Fatalf("namespace = %q", got)
	}
	if got := id.RuntimeCNIName(); got != "acme-core-net" {
		t.Fatalf("cni name = %q", got)
	}
	if got := id.RuntimeCNIBridge(); got != "esb-acme2467a7" {
		t.Fatalf("bridge = %q", got)
	}
	if got := id.RuntimeCNISubnet(); got != "10.104.4.0/23" {
		t.Fatalf("subnet = %q", got)
	}
	if got := id.RuntimeResolvConfPath(); got != "/run/containerd/acme-core/resolv.conf" {
		t.Fatalf("resolv path = %q", got)
	}
	if got := id.RuntimeContainerPrefix(); got != "acme-core" {
		t.Fatalf("container prefix = %q", got)
	}
	if got := id.ImagePrefix(); got != "acme-core" {
		t.Fatalf("image prefix = %q", got)
	}
	if got := id.EnvPrefix(); got != "ACME_CORE" {
		t.Fatalf("env prefix = %q", got)
	}
	if got := id.LabelPrefix(); got != "com.acme-core" {
		t.Fatalf("label prefix = %q", got)
	}
	if got := id.RuntimeLabelEnv(); got != "acme-core_env" {
		t.Fatalf("label env = %q", got)
	}
	if got := id.RuntimeLabelFunction(); got != "acme-core_function" {
		t.Fatalf("label function = %q", got)
	}
	if got := id.RuntimeLabelCreatedBy(); got != "created_by" {
		t.Fatalf("label created_by = %q", got)
	}
	if got := id.RuntimeLabelCreatedByValue(); got != "acme-core-agent" {
		t.Fatalf("label created_by value = %q", got)
	}
}

func TestStackIdentityEsbUsesDerivedValues(t *testing.T) {
	id := StackIdentity{BrandSlug: "esb"}
	if got := id.RuntimeCNIName(); got != "esb-net" {
		t.Fatalf("cni name = %q", got)
	}
	if got := id.RuntimeCNIBridge(); got != "esb-esbefb03b" {
		t.Fatalf("bridge = %q", got)
	}
	if got := id.RuntimeCNISubnet(); got != "10.184.208.0/23" {
		t.Fatalf("subnet = %q", got)
	}
}

func TestStackIdentityBrandIsolationDerivation(t *testing.T) {
	brandA := StackIdentity{BrandSlug: "brand-a"}
	brandB := StackIdentity{BrandSlug: "brand-b"}

	if brandA.RuntimeCNIName() == brandB.RuntimeCNIName() {
		t.Fatalf("expected distinct CNI names: %q", brandA.RuntimeCNIName())
	}
	if brandA.RuntimeCNIBridge() == brandB.RuntimeCNIBridge() {
		t.Fatalf("expected distinct bridges: %q", brandA.RuntimeCNIBridge())
	}
	if brandA.RuntimeCNISubnet() == brandB.RuntimeCNISubnet() {
		t.Fatalf("expected distinct subnets: %q", brandA.RuntimeCNISubnet())
	}
}

func TestStackIdentitySubnetShape(t *testing.T) {
	id := StackIdentity{BrandSlug: "shape-check"}
	subnet := id.RuntimeCNISubnet()

	if !strings.HasSuffix(subnet, "/23") {
		t.Fatalf("expected /23 subnet, got %q", subnet)
	}
	ip, cidr, err := net.ParseCIDR(subnet)
	if err != nil {
		t.Fatalf("parse subnet: %v", err)
	}
	ip4 := ip.To4()
	if ip4 == nil {
		t.Fatalf("expected IPv4 subnet, got %q", subnet)
	}
	if ip4[0] != 10 {
		t.Fatalf("expected 10.x subnet, got %q", subnet)
	}
	if ip4[1] == 88 {
		t.Fatalf("second octet 88 must be excluded: %q", subnet)
	}
	if ip4[2]%2 != 0 {
		t.Fatalf("third octet must be /23-aligned (even), got %q", subnet)
	}
	if ones, _ := cidr.Mask.Size(); ones != 23 {
		t.Fatalf("expected /23 mask, got %q", subnet)
	}
}

func TestStackIdentitySubnetProbeProgressionAndWrap(t *testing.T) {
	id := StackIdentity{BrandSlug: "probe-check"}
	base := id.RuntimeCNISubnetAt(0)
	if got := id.RuntimeCNISubnet(); got != base {
		t.Fatalf("RuntimeCNISubnet() = %q, want %q", got, base)
	}

	next := id.RuntimeCNISubnetAt(1)
	if next == base {
		t.Fatalf("expected next probe subnet to differ from base %q", base)
	}

	pool := RuntimeCNISubnetPoolSize()
	if pool <= 1 {
		t.Fatalf("invalid pool size: %d", pool)
	}
	if wrapped := id.RuntimeCNISubnetAt(pool); wrapped != base {
		t.Fatalf("expected wrap at pool size %d: got %q, want %q", pool, wrapped, base)
	}
}
