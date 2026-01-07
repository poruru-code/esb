// Where: services/agent/cmd/agent/cni_config.go
// What: Helpers to rewrite CNI config with a per-node IP allocation range.
// Why: Align worker IPs to wg_subnet while keeping the 10.88.0.1 gateway intact.
package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func prepareCNIConfig(baseFile, subnet string) (string, error) {
	baseConfig, err := os.ReadFile(baseFile)
	if err != nil {
		return "", fmt.Errorf("read CNI config: %w", err)
	}

	updatedConfig, err := applyCNISubnet(baseConfig, subnet)
	if err != nil {
		return "", err
	}

	outputDir := "/run/esb/cni"
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create CNI override dir: %w", err)
	}

	outputFile := filepath.Join(outputDir, filepath.Base(baseFile))
	if err := os.WriteFile(outputFile, append(updatedConfig, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("write CNI override file: %w", err)
	}

	return outputFile, nil
}

func applyCNISubnet(config []byte, subnet string) ([]byte, error) {
	cidr, err := parseIPv4CIDR(subnet)
	if err != nil {
		return nil, fmt.Errorf("invalid CNI_SUBNET: %w", err)
	}

	var root map[string]any
	if err := json.Unmarshal(config, &root); err != nil {
		return nil, fmt.Errorf("parse CNI config JSON: %w", err)
	}

	plugins, ok := root["plugins"].([]any)
	if !ok || len(plugins) == 0 {
		return nil, fmt.Errorf("CNI config missing plugins")
	}

	updated := false
	for i, plugin := range plugins {
		pluginMap, ok := plugin.(map[string]any)
		if !ok {
			continue
		}
		if pluginMap["type"] != "bridge" {
			continue
		}
		ipam, ok := pluginMap["ipam"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("bridge plugin missing ipam config")
		}
		baseSubnetRaw, ok := ipam["subnet"].(string)
		if !ok || baseSubnetRaw == "" {
			return nil, fmt.Errorf("bridge ipam subnet is missing")
		}

		baseSubnet, err := parseIPv4CIDR(baseSubnetRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid base subnet %q: %w", baseSubnetRaw, err)
		}

		rangeStart, rangeEnd, err := cidrHostRange(cidr)
		if err != nil {
			return nil, err
		}
		if !baseSubnet.Contains(rangeStart) || !baseSubnet.Contains(rangeEnd) {
			return nil, fmt.Errorf("CNI_SUBNET %s is outside base subnet %s", subnet, baseSubnetRaw)
		}

		ipam["ranges"] = []any{
			[]any{
				map[string]any{
					"subnet":     baseSubnetRaw,
					"rangeStart": rangeStart.String(),
					"rangeEnd":   rangeEnd.String(),
				},
			},
		}
		delete(ipam, "subnet")
		delete(ipam, "rangeStart")
		delete(ipam, "rangeEnd")
		pluginMap["ipam"] = ipam

		// Inject DNS settings
		pluginMap["dns"] = map[string]any{
			"nameservers": []string{"10.88.0.1"},
		}

		plugins[i] = pluginMap
		updated = true
		break
	}

	if !updated {
		return nil, fmt.Errorf("bridge plugin not found for CNI config")
	}

	root["plugins"] = plugins
	updatedConfig, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode CNI config JSON: %w", err)
	}
	return updatedConfig, nil
}

func parseIPv4CIDR(value string) (*net.IPNet, error) {
	_, cidr, err := net.ParseCIDR(strings.TrimSpace(value))
	if err != nil {
		return nil, err
	}
	if cidr == nil || cidr.IP.To4() == nil {
		return nil, fmt.Errorf("IPv4 CIDR required")
	}
	return cidr, nil
}

func cidrHostRange(cidr *net.IPNet) (net.IP, net.IP, error) {
	ones, bits := cidr.Mask.Size()
	if bits != 32 || cidr.IP.To4() == nil {
		return nil, nil, fmt.Errorf("IPv4 CIDR required")
	}
	if ones >= 31 {
		return nil, nil, fmt.Errorf("CIDR %s has no usable host range", cidr.String())
	}

	network := cidr.IP.Mask(cidr.Mask).To4()
	networkInt := ipv4ToUint32(network)
	maskInt := binary.BigEndian.Uint32(cidr.Mask)
	broadcastInt := networkInt | ^maskInt

	start := uint32ToIPv4(networkInt + 1)
	end := uint32ToIPv4(broadcastInt - 1)
	if start == nil || end == nil {
		return nil, nil, fmt.Errorf("failed to compute host range for %s", cidr.String())
	}
	return start, end, nil
}

func ipv4ToUint32(ip net.IP) uint32 {
	return binary.BigEndian.Uint32(ip.To4())
}

func uint32ToIPv4(value uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, value)
	return ip
}
