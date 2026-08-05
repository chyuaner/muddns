package ipfetcher

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// CalculateIPv4Offset takes a base IPv4 (e.g. 140.112.10.1) and replaces the last octet or adds offset
func CalculateIPv4Offset(baseIPStr string, offsetStr string) (string, error) {
	ip := net.ParseIP(baseIPStr)
	if ip == nil {
		return "", fmt.Errorf("invalid base ip: %s", baseIPStr)
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return "", fmt.Errorf("not an ipv4 address: %s", baseIPStr)
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		return "", fmt.Errorf("invalid offset: %s", offsetStr)
	}

	if offset >= 0 && offset <= 255 {
		// Set last octet directly
		return fmt.Sprintf("%d.%d.%d.%d", ip4[0], ip4[1], ip4[2], offset), nil
	}

	// Otherwise add offset to last octet
	lastOctet := int(ip4[3]) + offset
	if lastOctet < 0 || lastOctet > 255 {
		return "", fmt.Errorf("offset result out of range: %d", lastOctet)
	}

	return fmt.Sprintf("%d.%d.%d.%d", ip4[0], ip4[1], ip4[2], lastOctet), nil
}

// FetchARPMAC inspects /proc/net/arp to find public IPv4 for a MAC address
func FetchARPMAC(targetMAC string) (string, error) {
	data, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		return "", fmt.Errorf("failed to read /proc/net/arp: %w", err)
	}

	normalizedTarget := strings.ToLower(strings.ReplaceAll(targetMAC, "-", ":"))
	lines := strings.Split(string(data), "\n")

	for _, line := range lines[1:] { // skip header
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			ip := fields[0]
			mac := strings.ToLower(fields[3])
			if mac == normalizedTarget {
				return ip, nil
			}
		}
	}

	return "", fmt.Errorf("mac address %s not found in arp table", targetMAC)
}
