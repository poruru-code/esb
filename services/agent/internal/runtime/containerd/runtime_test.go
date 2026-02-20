package containerd

import (
	"net"
	"testing"

	"github.com/containerd/go-cni"
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

func TestNewRuntime_UsesSharedCNINetwork(t *testing.T) {
	rt := NewRuntime(nil, nil, "acme", "dev", "brand-a")
	if got := rt.cniNetwork; got != "esb-net" {
		t.Fatalf("cniNetwork = %q, want %q", got, "esb-net")
	}

	rtOther := NewRuntime(nil, nil, "acme", "dev", "brand-b")
	if got := rtOther.cniNetwork; got != "esb-net" {
		t.Fatalf("cniNetwork (other brand) = %q, want %q", got, "esb-net")
	}
}
