package ipfetcher

import (
	"fmt"
	"net"
)

// StitchIPv6 combines raw IPv6 prefix (first 64 bits) with a suffix (last 64 bits, e.g. ::100)
func StitchIPv6(rawIPStr string, suffixStr string) (string, error) {
	ip := net.ParseIP(rawIPStr)
	if ip == nil || ip.To4() != nil {
		return "", fmt.Errorf("invalid IPv6 prefix source: %s", rawIPStr)
	}

	ipBytes := ip.To16()
	prefixBytes := ipBytes[:8]

	suffixIP := net.ParseIP(suffixStr)
	if suffixIP == nil {
		return "", fmt.Errorf("invalid IPv6 suffix: %s", suffixStr)
	}
	suffixBytes := suffixIP.To16()

	finalBytes := make([]byte, 16)
	copy(finalBytes[:8], prefixBytes)
	copy(finalBytes[8:], suffixBytes[8:])

	finalIP := net.IP(finalBytes)
	return finalIP.String(), nil
}

// ConvertMACToEUI64 converts MAC (00:11:22:33:44:55) into IPv6 suffix ::211:22ff:fe33:4455
func ConvertMACToEUI64(macStr string) (string, error) {
	hw, err := net.ParseMAC(macStr)
	if err != nil {
		return "", fmt.Errorf("invalid mac address: %w", err)
	}
	if len(hw) != 6 {
		return "", fmt.Errorf("mac address must be 6 bytes (got %d)", len(hw))
	}

	// Flip 7th bit of 1st byte (universal/local bit)
	firstByte := hw[0] ^ 0x02

	eui64Bytes := []byte{
		firstByte, hw[1], hw[2], 0xff, 0xfe, hw[3], hw[4], hw[5],
	}

	finalBytes := make([]byte, 16)
	copy(finalBytes[8:], eui64Bytes)

	suffixIP := net.IP(finalBytes)
	return suffixIP.String(), nil
}

// StitchIPv6WithMAC combines raw IPv6 prefix with MAC address converted via EUI-64
func StitchIPv6WithMAC(rawIPStr string, macStr string) (string, error) {
	suffix, err := ConvertMACToEUI64(macStr)
	if err != nil {
		return "", err
	}
	return StitchIPv6(rawIPStr, suffix)
}
