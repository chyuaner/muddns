package ipfetcher

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

func FetchInterfaceIP(ifaceName string, matchPattern string, isIPv6 bool) (string, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return "", fmt.Errorf("interface %s not found: %w", ifaceName, err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return "", fmt.Errorf("failed to get addrs for %s: %w", ifaceName, err)
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

		if ip == nil {
			continue
		}

		if isIPv6 {
			// Skip IPv4 or link-local (fe80::)
			if ip.To4() == nil && !ip.IsLinkLocalUnicast() && !ip.IsLoopback() {
				candidates = append(candidates, ip.String())
			}
		} else {
			// IPv4 only
			if ip4 := ip.To4(); ip4 != nil && !ip.IsLoopback() {
				candidates = append(candidates, ip4.String())
			}
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no valid IP found on interface %s (isIPv6=%v)", ifaceName, isIPv6)
	}

	// 1. Handle index matching (@1, @2, @3)
	if strings.HasPrefix(matchPattern, "@") {
		idxStr := strings.TrimPrefix(matchPattern, "@")
		idx, err := strconv.Atoi(idxStr)
		if err == nil && idx >= 1 && idx <= len(candidates) {
			return candidates[idx-1], nil
		}
	}

	// 2. Handle regex matching
	if matchPattern != "" && !strings.HasPrefix(matchPattern, "@") {
		re, err := regexp.Compile(matchPattern)
		if err == nil {
			for _, c := range candidates {
				if re.MatchString(c) {
					return c, nil
				}
			}
		}
	}

	// Default: return first candidate
	return candidates[0], nil
}
