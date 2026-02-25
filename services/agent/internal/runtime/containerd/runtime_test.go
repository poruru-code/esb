package containerd

import (
	"net"
	"testing"

	"github.com/containerd/go-cni"
	"github.com/poruru-code/esb/services/agent/internal/config"
)

func TestExtractIPv4(t *testing.T) {
	tests := []struct {
		name    string
		result  *cni.Result
		want    string
		wantErr bool
	}{
		{
			name: "Scenario A: Gateway (Sandbox empty) present. Returns IP (current behavior).",
			result: &cni.Result{
				Interfaces: map[string]*cni.Config{
					"cni0": { // Gateway
						Sandbox: "",
						IPConfigs: []*cni.IPConfig{
							{IP: net.ParseIP("10.88.1.1")},
						},
					},
				},
			},
			// Current implementation returns any IPv4 found regardless of Sandbox.
			want:    "10.88.1.1",
			wantErr: false,
		},
		{
			name: "Scenario B: Container (Sandbox set). Should be returned.",
			result: &cni.Result{
				Interfaces: map[string]*cni.Config{
					"eth0": { // Container
						Sandbox: "/var/run/netns/test",
						IPConfigs: []*cni.IPConfig{
							{IP: net.ParseIP("10.88.1.3")},
						},
					},
				},
			},
			want:    "10.88.1.3",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractIPv4(tt.result)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractIPv4() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractIPv4() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewRuntime_UsesBrandScopedCNINetwork(t *testing.T) {
	rt := NewRuntime(nil, nil, "acme", "dev", "brand-a")
	if got := rt.cniNetwork; got != "brand-a-net" {
		t.Fatalf("cniNetwork = %q, want %q", got, "brand-a-net")
	}

	rtOther := NewRuntime(nil, nil, "acme", "dev", "brand-b")
	if got := rtOther.cniNetwork; got != "brand-b-net" {
		t.Fatalf("cniNetwork (other brand) = %q, want %q", got, "brand-b-net")
	}
}

func TestResolveCNIDNSServer_FromSubnet(t *testing.T) {
	t.Setenv("CNI_DNS_SERVER", "")
	t.Setenv("CNI_GW_IP", "")
	t.Setenv("CNI_SUBNET", "10.44.16.0/20")

	if got := resolveCNIDNSServer(); got != "10.44.16.1" {
		t.Fatalf("resolveCNIDNSServer() = %q, want %q", got, "10.44.16.1")
	}
}

func TestResolveCNIDNSServer_DefaultFallback(t *testing.T) {
	t.Setenv("CNI_DNS_SERVER", "")
	t.Setenv("CNI_GW_IP", "")
	t.Setenv("CNI_SUBNET", "invalid-cidr")

	if got := resolveCNIDNSServer(); got != config.DefaultCNIDNSServer {
		t.Fatalf("resolveCNIDNSServer() = %q, want %q", got, config.DefaultCNIDNSServer)
	}
}

func TestNewRuntime_PanicsWhenNamespaceMissing(t *testing.T) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic")
		}
	}()
	_ = NewRuntime(nil, nil, "", "dev", "brand-a")
}

func TestNewRuntime_PanicsWhenBrandInvalid(t *testing.T) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic")
		}
	}()
	_ = NewRuntime(nil, nil, "acme", "dev", "___")
}
