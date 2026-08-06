package ipfetcher

import (
	"fmt"
	"net"
	"strings"
)

// StitchIPv6 取得 IPv6 的 /64 前綴，並拼接指定的固定後綴 (例如 prefix: 2001:db8::, suffix: ::100 -> 2001:db8::100)
func StitchIPv6(prefixIPStr string, suffixStr string) (string, error) {
	prefixIP := net.ParseIP(strings.TrimSpace(prefixIPStr)).To16()
	if prefixIP == nil {
		return "", fmt.Errorf("無效的前綴 IPv6 地址: %s", prefixIPStr)
	}

	suffixIP := net.ParseIP(strings.TrimSpace(suffixStr)).To16()
	if suffixIP == nil {
		return "", fmt.Errorf("無效的後綴 IPv6 地址: %s", suffixStr)
	}

	result := make(net.IP, 16)
	// 保留前 8 個 Byte (64 bit) 前綴
	copy(result[0:8], prefixIP[0:8])
	// 覆蓋後 8 個 Byte (64 bit) 後綴
	copy(result[8:16], suffixIP[8:16])

	return result.String(), nil
}

// StitchIPv6WithMAC 取得 IPv6 前綴並搭配 MAC 地址算出 EUI-64 後綴
func StitchIPv6WithMAC(prefixIPStr string, macStr string) (string, error) {
	hw, err := net.ParseMAC(strings.TrimSpace(macStr))
	if err != nil || len(hw) != 6 {
		return "", fmt.Errorf("無效的 MAC 地址 %q: %v", macStr, err)
	}

	prefixIP := net.ParseIP(strings.TrimSpace(prefixIPStr)).To16()
	if prefixIP == nil {
		return "", fmt.Errorf("無效的前綴 IPv6 地址: %s", prefixIPStr)
	}

	// 根據 EUI-64 規範計算：反轉第 7 位元 (Universal/Local bit)，中間插入 0xfffe
	eui64 := make([]byte, 8)
	eui64[0] = hw[0] ^ 0x02
	eui64[1] = hw[1]
	eui64[2] = hw[2]
	eui64[3] = 0xff
	eui64[4] = 0xfe
	eui64[5] = hw[3]
	eui64[6] = hw[4]
	eui64[7] = hw[5]

	result := make(net.IP, 16)
	copy(result[0:8], prefixIP[0:8])
	copy(result[8:16], eui64)

	return result.String(), nil
}
