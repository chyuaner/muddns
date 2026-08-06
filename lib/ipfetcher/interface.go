package ipfetcher

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// FetchInterfaceIP 自指定系統網卡 (如 eth0, wg0, eth1@if75) 讀取對應的 IPv4 或 IPv6 地址
// 支援使用索引 (例 `@1`, `@2`) 取得第 N 個匹配的 IP，或是正則表達式進行篩選
func FetchInterfaceIP(ifaceName string, matchExpr string, isIPv6 bool) (string, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil && strings.Contains(ifaceName, "@") {
		// LXC / veth 介面名稱在 ip addr 常顯示為 eth1@if75，但 OS 實際介面名為 eth1
		cleanName := strings.Split(ifaceName, "@")[0]
		if cleanIface, cleanErr := net.InterfaceByName(cleanName); cleanErr == nil {
			iface = cleanIface
			err = nil
		}
	}
	if err != nil {
		return "", fmt.Errorf("找不到網卡介面 %s: %w", ifaceName, err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return "", fmt.Errorf("無法取得網卡 %s 的 IP 地址: %w", ifaceName, err)
	}

	var candidates []string
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}

		if ip == nil || ip.IsLoopback() {
			continue
		}

		// 根據類型篩選
		if isIPv6 {
			// 過濾 Link-Local (fe80::) 區域無效地址
			if ip.To4() == nil && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() {
				candidates = append(candidates, ip.String())
			}
		} else {
			if ip.To4() != nil {
				candidates = append(candidates, ip.String())
			}
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("網卡 %s 上找不到符合條件的 %s 地址", ifaceName, map[bool]string{false: "IPv4", true: "IPv6"}[isIPv6])
	}

	// 處理索引語法 (例如 @1 代表第一個 IP，@2 代表第二個 IP)
	if strings.HasPrefix(matchExpr, "@") {
		idxStr := strings.TrimPrefix(matchExpr, "@")
		idx, err := strconv.Atoi(idxStr)
		if err == nil && idx > 0 && idx <= len(candidates) {
			return candidates[idx-1], nil
		}
		return "", fmt.Errorf("索引 @%s 超出網卡候選 IP 範圍 (共有 %d 個)", idxStr, len(candidates))
	}

	// 處理正則表達式篩選
	if matchExpr != "" {
		re, err := regexp.Compile(matchExpr)
		if err != nil {
			return "", fmt.Errorf("無效的正則表達式 %q: %w", matchExpr, err)
		}
		for _, cand := range candidates {
			if re.MatchString(cand) {
				return cand, nil
			}
		}
		return "", fmt.Errorf("網卡 %s 上沒有 IP 匹配正則條件 %q", ifaceName, matchExpr)
	}

	// 預設傳回第一個合格的 IP
	return candidates[0], nil
}
