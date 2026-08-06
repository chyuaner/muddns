package ipfetcher

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

// CalculateIPv4Offset 根據基底 IPv4 (如 140.112.1.1) 加上數值偏移量 (如 10) 算出最終 IP (140.112.1.11)
func CalculateIPv4Offset(baseIPStr string, offsetStr string) (string, error) {
	ip := net.ParseIP(strings.TrimSpace(baseIPStr)).To4()
	if ip == nil {
		return "", fmt.Errorf("無效的基底 IPv4 地址: %s", baseIPStr)
	}

	offset, err := strconv.Atoi(strings.TrimSpace(offsetStr))
	if err != nil {
		return "", fmt.Errorf("無效的偏移量數值: %s", offsetStr)
	}

	// 轉為 uint32 進行加法運算
	ipUint := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
	resultUint := ipUint + uint32(offset)

	resultIP := net.IPv4(
		byte(resultUint>>24),
		byte(resultUint>>16),
		byte(resultUint>>8),
		byte(resultUint),
	)

	return resultIP.String(), nil
}

// FetchARPMAC 自系統 ARP / 鄰居表中，透過指定 MAC 地址反查對應的 IPv4 地址
func FetchARPMAC(macStr string) (string, error) {
	macStr = strings.ToLower(strings.TrimSpace(macStr))
	if macStr == "" {
		return "", fmt.Errorf("未指定 MAC 地址")
	}

	cmd := exec.Command("ip", "neighbor", "show")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("執行 ip neighbor 失敗: %w", err)
	}

	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		lineLower := strings.ToLower(line)
		if strings.Contains(lineLower, macStr) {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				parsedIP := net.ParseIP(fields[0])
				if parsedIP != nil && parsedIP.To4() != nil {
					return parsedIP.String(), nil
				}
			}
		}
	}

	return "", fmt.Errorf("在 ARP / 鄰居表中找不到 MAC %s 對應的 IPv4", macStr)
}
