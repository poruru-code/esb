package maven

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Endpoint represents one proxy endpoint.
type Endpoint struct {
	Scheme   string
	Host     string
	Port     int
	Username string
	Password string
}

// Endpoints groups HTTP and HTTPS proxy endpoints.
type Endpoints struct {
	HTTP  *Endpoint
	HTTPS *Endpoint
}

var proxyURLPattern = regexp.MustCompile(`^(?i)(http|https)://([^/@[:space:]]+(:[^@[:space:]]*)?@)?(\[[^]]+\]|[^/:?#[:space:]]+)(:([0-9]+))?/?$`)

func resolveAlias(env map[string]string, upper, lower string) string {
	if value := strings.TrimSpace(env[upper]); value != "" {
		return value
	}
	return strings.TrimSpace(env[lower])
}

func parseProxyURL(raw, label string) (*Endpoint, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	match := proxyURLPattern.FindStringSubmatch(trimmed)
	if match == nil {
		return nil, fmt.Errorf("invalid proxy URL for %s: %s", label, raw)
	}

	scheme := strings.ToLower(match[1])
	userinfo := match[2]
	host := match[4]
	portRaw := match[6]
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	if host == "" {
		return nil, fmt.Errorf("invalid proxy URL for %s: %s", label, raw)
	}

	port := 0
	if portRaw == "" {
		if scheme == "https" {
			port = 443
		} else {
			port = 80
		}
	} else {
		parsedPort, err := strconv.Atoi(portRaw)
		if err != nil || parsedPort < 1 || parsedPort > 65535 {
			return nil, fmt.Errorf("invalid proxy URL port for %s: %s", label, raw)
		}
		port = parsedPort
	}

	username := ""
	password := ""
	if userinfo != "" {
		userinfo = strings.TrimSuffix(userinfo, "@")
		if user, pass, ok := strings.Cut(userinfo, ":"); ok {
			decodedUser, userErr := url.QueryUnescape(user)
			if userErr != nil {
				decodedUser = user
			}
			decodedPass, passErr := url.QueryUnescape(pass)
			if passErr != nil {
				decodedPass = pass
			}
			username = decodedUser
			password = decodedPass
		} else {
			decodedUser, userErr := url.QueryUnescape(userinfo)
			if userErr != nil {
				decodedUser = userinfo
			}
			username = decodedUser
		}
	}

	return &Endpoint{
		Scheme:   scheme,
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
	}, nil
}

// ResolveEndpointsFromEnv resolves proxy endpoints from environment aliases.
// HTTPS falls back to HTTP only when HTTPS is missing.
func ResolveEndpointsFromEnv(env map[string]string) (Endpoints, error) {
	httpRaw := resolveAlias(env, "HTTP_PROXY", "http_proxy")
	httpsRaw := resolveAlias(env, "HTTPS_PROXY", "https_proxy")

	httpEndpoint, err := parseProxyURL(httpRaw, "HTTP_PROXY/http_proxy")
	if err != nil {
		return Endpoints{}, err
	}
	httpsEndpoint, err := parseProxyURL(httpsRaw, "HTTPS_PROXY/https_proxy")
	if err != nil {
		return Endpoints{}, err
	}
	if httpsEndpoint == nil && httpEndpoint != nil {
		copyEndpoint := *httpEndpoint
		httpsEndpoint = &copyEndpoint
	}
	return Endpoints{HTTP: httpEndpoint, HTTPS: httpsEndpoint}, nil
}
