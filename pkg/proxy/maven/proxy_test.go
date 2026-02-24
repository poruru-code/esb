package maven

import "testing"

func TestResolveEndpointsFromEnv_Empty(t *testing.T) {
	endpoints, err := ResolveEndpointsFromEnv(map[string]string{})
	if err != nil {
		t.Fatalf("ResolveEndpointsFromEnv() error = %v", err)
	}
	if endpoints.HTTP != nil || endpoints.HTTPS != nil {
		t.Fatalf("expected nil endpoints, got %+v", endpoints)
	}
}

func TestResolveEndpointsFromEnv_HTTPAndFallbackHTTPS(t *testing.T) {
	endpoints, err := ResolveEndpointsFromEnv(map[string]string{
		"HTTP_PROXY": "http://user:pass@proxy.example:8080/",
	})
	if err != nil {
		t.Fatalf("ResolveEndpointsFromEnv() error = %v", err)
	}
	if endpoints.HTTP == nil || endpoints.HTTPS == nil {
		t.Fatalf("expected both endpoints, got %+v", endpoints)
	}
	if endpoints.HTTP.Host != "proxy.example" || endpoints.HTTP.Port != 8080 {
		t.Fatalf("unexpected HTTP endpoint: %+v", endpoints.HTTP)
	}
	if endpoints.HTTP.Username != "user" || endpoints.HTTP.Password != "pass" {
		t.Fatalf("unexpected HTTP auth: %+v", endpoints.HTTP)
	}
	if endpoints.HTTPS.Host != endpoints.HTTP.Host || endpoints.HTTPS.Port != endpoints.HTTP.Port {
		t.Fatalf("expected HTTPS fallback to HTTP, got %+v", endpoints.HTTPS)
	}
}

func TestResolveEndpointsFromEnv_SeparateHTTPS(t *testing.T) {
	endpoints, err := ResolveEndpointsFromEnv(map[string]string{
		"http_proxy":  "http://proxy-http.local:8080",
		"HTTPS_PROXY": "https://proxy-https.local:8443",
	})
	if err != nil {
		t.Fatalf("ResolveEndpointsFromEnv() error = %v", err)
	}
	if endpoints.HTTP == nil || endpoints.HTTPS == nil {
		t.Fatalf("expected both endpoints, got %+v", endpoints)
	}
	if endpoints.HTTP.Host != "proxy-http.local" || endpoints.HTTP.Port != 8080 {
		t.Fatalf("unexpected HTTP endpoint: %+v", endpoints.HTTP)
	}
	if endpoints.HTTPS.Host != "proxy-https.local" || endpoints.HTTPS.Port != 8443 {
		t.Fatalf("unexpected HTTPS endpoint: %+v", endpoints.HTTPS)
	}
}

func TestResolveEndpointsFromEnv_DecodeUserInfo(t *testing.T) {
	endpoints, err := ResolveEndpointsFromEnv(map[string]string{
		"HTTPS_PROXY": "https://web_user:Web%5FUser@proxy.example:8443",
	})
	if err != nil {
		t.Fatalf("ResolveEndpointsFromEnv() error = %v", err)
	}
	if endpoints.HTTPS == nil {
		t.Fatalf("expected https endpoint, got %+v", endpoints)
	}
	if endpoints.HTTPS.Username != "web_user" || endpoints.HTTPS.Password != "Web_User" {
		t.Fatalf("unexpected auth decode: %+v", endpoints.HTTPS)
	}
}

func TestResolveEndpointsFromEnv_InvalidProxyURL(t *testing.T) {
	_, err := ResolveEndpointsFromEnv(map[string]string{
		"HTTP_PROXY": "http://proxy.example:8080/path",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveEndpointsFromEnv_InvalidScheme(t *testing.T) {
	_, err := ResolveEndpointsFromEnv(map[string]string{
		"HTTP_PROXY": "socks5://proxy.example:8080",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveEndpointsFromEnv_InvalidPort(t *testing.T) {
	_, err := ResolveEndpointsFromEnv(map[string]string{
		"HTTP_PROXY": "http://proxy.example:99999",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
