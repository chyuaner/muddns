package ipfetcher

import (
	"fmt"
	"net"
	"os/exec"
	"strings"

	"muddns/config"
)

func ResolveIP(ipCfg config.IPConfig, isIPv6 bool, defaults []string, hostID string) (string, error) {
	if !ipCfg.Enabled {
		return "", nil
	}

	switch ipCfg.Mode {
	case "external_api":
		return FetchExternalIP(ipCfg.URL, isIPv6, defaults, hostID)

	case "command":
		if strings.TrimSpace(ipCfg.Command) == "" {
			return "", fmt.Errorf("command 模式下未提供 bash 指令")
		}
		cmd := exec.Command("bash", "-c", ipCfg.Command)
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("執行指令失敗: %w", err)
		}
		ipStr := strings.TrimSpace(string(out))
		parsedIP := net.ParseIP(ipStr)
		if parsedIP == nil {
			return "", fmt.Errorf("指令輸出內容 %q 不是有效的 IP 地址", ipStr)
		}
		if isIPv6 && parsedIP.To4() != nil {
			return "", fmt.Errorf("指令輸出 %q 為 IPv4，預期為 IPv6", ipStr)
		}
		if !isIPv6 && parsedIP.To4() == nil {
			return "", fmt.Errorf("指令輸出 %q 為 IPv6，預期為 IPv4", ipStr)
		}
		return ipStr, nil

	case "interface":
		iface := ipCfg.Interface
		if iface == "" {
			iface = "eth0"
		}
		return FetchInterfaceIP(iface, ipCfg.Match, isIPv6)

	case "base_offset":
		if isIPv6 {
			return "", fmt.Errorf("base_offset mode is only for IPv4")
		}
		iface := ipCfg.Interface
		if iface == "" {
			iface = "eth0"
		}
		baseIP, err := FetchInterfaceIP(iface, "", false)
		if err != nil {
			return "", err
		}
		return CalculateIPv4Offset(baseIP, ipCfg.Offset)

	case "arp_mac":
		if isIPv6 {
			return "", fmt.Errorf("arp_mac mode is only for IPv4")
		}
		return FetchARPMAC(ipCfg.MAC)

	case "prefix_stitching":
		if !isIPv6 {
			return "", fmt.Errorf("prefix_stitching mode is only for IPv6")
		}
		iface := ipCfg.Interface
		if iface == "" {
			iface = "eth0"
		}
		prefixIP, err := FetchInterfaceIP(iface, "", true)
		if err != nil {
			return "", err
		}
		suffix := ipCfg.Suffix
		if suffix == "" {
			suffix = "::1"
		}
		return StitchIPv6(prefixIP, suffix)

	case "eui64_mac":
		if !isIPv6 {
			return "", fmt.Errorf("eui64_mac mode is only for IPv6")
		}
		iface := ipCfg.Interface
		if iface == "" {
			iface = "eth0"
		}
		prefixIP, err := FetchInterfaceIP(iface, "", true)
		if err != nil {
			return "", err
		}
		return StitchIPv6WithMAC(prefixIP, ipCfg.MAC)

	default:
		return "", fmt.Errorf("unknown IP mode: %s", ipCfg.Mode)
	}
}
