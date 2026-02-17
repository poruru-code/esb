package identity

import "testing"

func TestResolveStackIdentityFrom(t *testing.T) {
	tests := []struct {
		name              string
		brandSlug         string
		projectName       string
		envName           string
		containersNetwork string
		wantBrand         string
		wantSource        string
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
			name:       "fallback default",
			wantBrand:  "esb",
			wantSource: "fallback",
		},
		{
			name:              "default bridge network is ignored",
			containersNetwork: "bridge",
			wantBrand:         "esb",
			wantSource:        "fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveStackIdentityFrom(tt.brandSlug, tt.projectName, tt.envName, tt.containersNetwork)
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
	if got := id.RuntimeCNIBridge(); got != "acme-core0" {
		t.Fatalf("bridge = %q", got)
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
