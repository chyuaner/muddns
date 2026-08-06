// Package ipfetcher 負責計算與取得主機的公網 IPv4 與 IPv6 地址。
// 支援外部 Echo API 查詢、網卡 IP 讀取、網卡索引匹配、學術網段偏移量計算、ARP 鄰居表探測、IPv6 動態前綴拼接、EUI-64 MAC 計算以及自訂 Bash 指令輸出。
package ipfetcher

import (
	"fmt"
	"net"
	"os/exec"
	"strings"

	"muddns/config"
)

// ResolveIP 根據傳入的 IPConfig 策略與模式，派發計算出目標 IP 地址
//
// 參數:
//   - ipCfg: 包含模式與參數的設定結構
//   - isIPv6: 標示當前計算是否為 IPv6 策略
//   - defaults: 預設外網 Echo API 清單
//   - hostID: 關聯的主機識別名稱
//
// 回傳:
//   - string: 計算所得的 IP 字串
//   - error: 計算過程發生的錯誤
func ResolveIP(ipCfg config.IPConfig, isIPv6 bool, defaults []string, hostID string) (string, error) {
	if !ipCfg.Enabled {
		return "", nil
	}

	switch ipCfg.Mode {
	case "external_api":
		// 使用外網 API 查詢 IP
		return FetchExternalIP(ipCfg.URL, isIPv6, defaults, hostID)

	case "command":
		// 執行自訂 Bash 指令並解析 stdout 為 IP
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
		// 直接自指定網卡介面讀取 IP
		iface := ipCfg.Interface
		if iface == "" {
			iface = "eth0"
		}
		return FetchInterfaceIP(iface, ipCfg.Match, isIPv6)

	case "base_offset":
		// 學術網段模式：取得基礎 IP 並加上整數偏移量 (僅限 IPv4)
		if isIPv6 {
			return "", fmt.Errorf("base_offset 模式僅支援 IPv4")
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
		// ARP / 鄰居表 MAC 探測模式 (僅限 IPv4)
		if isIPv6 {
			return "", fmt.Errorf("arp_mac 模式僅支援 IPv4")
		}
		return FetchARPMAC(ipCfg.MAC)

	case "prefix_stitching":
		// IPv6 前綴拼接模式：取得網卡動態 /64 前綴，拼接自訂固定後綴
		if !isIPv6 {
			return "", fmt.Errorf("prefix_stitching 模式僅支援 IPv6")
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
		// IPv6 EUI-64 模式：取得網卡動態 /64 前綴，搭配指定 MAC 地址計算 EUI-64 後綴
		if !isIPv6 {
			return "", fmt.Errorf("eui64_mac 模式僅支援 IPv6")
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
		return "", fmt.Errorf("未知的 IP 計算模式: %s", ipCfg.Mode)
	}
}
