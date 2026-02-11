// Where: cli/internal/infra/templategen/stage_java_maven.go
// What: Maven proxy/settings helpers for Java runtime jar builds.
// Why: Keep proxy parsing and XML rendering isolated from runtime staging flow.
package templategen

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type mavenProxyEndpoint struct {
	Host     string
	Port     int
	Username string
	Password string
}

func firstConfiguredEnv(keys ...string) (string, bool) {
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		return trimmed, true
	}
	return "", false
}

func appendJavaBuildEnvArgs(args []string) []string {
	for _, key := range []string{
		"HTTP_PROXY",
		"http_proxy",
		"HTTPS_PROXY",
		"https_proxy",
		"NO_PROXY",
		"no_proxy",
		"MAVEN_OPTS",
		"JAVA_TOOL_OPTIONS",
	} {
		args = append(args, "-e", key+"=")
	}
	return args
}

func validateMavenProxyURL(envLabel string, rawURL string) error {
	value := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", envLabel, err)
	}
	if parsed.Scheme == "" || parsed.Hostname() == "" {
		return fmt.Errorf("%s must include scheme and host: %s", envLabel, rawURL)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%s must use http or https scheme: %s", envLabel, rawURL)
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf(
			"%s must not include path/query/fragment: %s",
			envLabel,
			rawURL,
		)
	}
	if portText := parsed.Port(); portText != "" {
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("%s has invalid port: %s", envLabel, rawURL)
		}
	}
	return nil
}

func parseMavenProxyEndpoint(envLabel string, rawURL string) (mavenProxyEndpoint, error) {
	if err := validateMavenProxyURL(envLabel, rawURL); err != nil {
		return mavenProxyEndpoint{}, err
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return mavenProxyEndpoint{}, fmt.Errorf("%s is invalid: %w", envLabel, err)
	}
	port := 0
	if parsed.Port() != "" {
		port, err = strconv.Atoi(parsed.Port())
		if err != nil || port < 1 || port > 65535 {
			return mavenProxyEndpoint{}, fmt.Errorf("%s has invalid port: %s", envLabel, rawURL)
		}
	} else {
		if strings.EqualFold(parsed.Scheme, "https") {
			port = 443
		} else {
			port = 80
		}
	}

	username := ""
	password := ""
	if parsed.User != nil {
		username = parsed.User.Username()
		if decoded, decodeErr := url.PathUnescape(username); decodeErr == nil {
			username = decoded
		}
		if rawPassword, ok := parsed.User.Password(); ok {
			password = rawPassword
			if decoded, decodeErr := url.PathUnescape(password); decodeErr == nil {
				password = decoded
			}
		}
	}

	return mavenProxyEndpoint{
		Host:     parsed.Hostname(),
		Port:     port,
		Username: username,
		Password: password,
	}, nil
}

func normalizeNoProxyToken(token string) string {
	normalized := strings.TrimSpace(token)
	if normalized == "" {
		return ""
	}

	if strings.HasPrefix(normalized, "[") {
		if closingIndex := strings.Index(normalized, "]"); closingIndex > 1 {
			normalized = strings.TrimSpace(normalized[1:closingIndex])
		}
	} else if strings.Count(normalized, ":") == 1 {
		host, port, ok := strings.Cut(normalized, ":")
		if ok {
			if _, err := strconv.Atoi(port); err == nil {
				normalized = strings.TrimSpace(host)
			}
		}
	}

	if strings.HasPrefix(normalized, ".") && !strings.HasPrefix(normalized, "*.") {
		normalized = "*" + normalized
	}
	return normalized
}

func buildMavenNonProxyHosts(raw string) string {
	seen := make(map[string]struct{})
	values := make([]string, 0)
	for _, token := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';'
	}) {
		normalized := normalizeNoProxyToken(token)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		values = append(values, normalized)
	}
	return strings.Join(values, "|")
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}

func renderMavenProxyBlock(
	proxyID string,
	protocol string,
	endpoint mavenProxyEndpoint,
	nonProxyHosts string,
) []string {
	lines := []string{
		"    <proxy>",
		fmt.Sprintf("      <id>%s</id>", xmlEscape(proxyID)),
		"      <active>true</active>",
		fmt.Sprintf("      <protocol>%s</protocol>", xmlEscape(protocol)),
		fmt.Sprintf("      <host>%s</host>", xmlEscape(endpoint.Host)),
		fmt.Sprintf("      <port>%d</port>", endpoint.Port),
	}
	if endpoint.Username != "" {
		lines = append(lines, fmt.Sprintf("      <username>%s</username>", xmlEscape(endpoint.Username)))
	}
	if endpoint.Password != "" {
		lines = append(lines, fmt.Sprintf("      <password>%s</password>", xmlEscape(endpoint.Password)))
	}
	if nonProxyHosts != "" {
		lines = append(
			lines,
			fmt.Sprintf("      <nonProxyHosts>%s</nonProxyHosts>", xmlEscape(nonProxyHosts)),
		)
	}
	lines = append(lines, "    </proxy>")
	return lines
}

func renderMavenSettingsXML() (string, error) {
	httpRaw, hasHTTP := firstConfiguredEnv("HTTP_PROXY", "http_proxy")
	httpsRaw, hasHTTPS := firstConfiguredEnv("HTTPS_PROXY", "https_proxy")

	var (
		httpEndpoint  *mavenProxyEndpoint
		httpsEndpoint *mavenProxyEndpoint
	)
	if hasHTTP {
		parsed, err := parseMavenProxyEndpoint("HTTP_PROXY/http_proxy", httpRaw)
		if err != nil {
			return "", err
		}
		httpEndpoint = &parsed
	}
	if hasHTTPS {
		parsed, err := parseMavenProxyEndpoint("HTTPS_PROXY/https_proxy", httpsRaw)
		if err != nil {
			return "", err
		}
		httpsEndpoint = &parsed
	}
	if httpsEndpoint == nil && httpEndpoint != nil {
		fallback := *httpEndpoint
		httpsEndpoint = &fallback
	}

	nonProxyHosts := ""
	if rawNoProxy, ok := firstConfiguredEnv("NO_PROXY", "no_proxy"); ok {
		nonProxyHosts = buildMavenNonProxyHosts(rawNoProxy)
	}

	lines := []string{
		"<settings>",
		"  <proxies>",
	}
	if httpEndpoint != nil {
		lines = append(lines, renderMavenProxyBlock("http-proxy", "http", *httpEndpoint, nonProxyHosts)...)
	}
	if httpsEndpoint != nil {
		lines = append(
			lines,
			renderMavenProxyBlock("https-proxy", "https", *httpsEndpoint, nonProxyHosts)...,
		)
	}
	lines = append(
		lines,
		"  </proxies>",
		"</settings>",
	)
	return strings.Join(lines, "\n") + "\n", nil
}

func writeTempMavenSettingsFile() (string, error) {
	settings, err := renderMavenSettingsXML()
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", "esb-m2-settings-*.xml")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if _, err := file.WriteString(settings); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}
