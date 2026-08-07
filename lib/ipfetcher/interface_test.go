package ipfetcher

import (
	"net"
	"testing"
)

func TestFetchInterfaceIP_Fallback(t *testing.T) {
	ifaces, err := net.Interfaces()
	if err != nil || len(ifaces) == 0 {
		t.Skip("無可用網絡介面進行測試")
	}

	var activeIface string
	for _, iface := range ifaces {
		if (iface.Flags&net.FlagUp) != 0 && (iface.Flags&net.FlagLoopback) == 0 {
			addrs, err := iface.Addrs()
			if err == nil && len(addrs) > 0 {
				activeIface = iface.Name
				break
			}
		}
	}

	if activeIface == "" {
		t.Skip("無非 Loopback 之活躍網卡可供測試")
	}

	// 測試帶有 LXC / veth 格式 @ 標記的介面名稱 (例如 eth0@if123)
	mockLxcName := activeIface + "@if123"
	ip, err := FetchInterfaceIP(mockLxcName, "", false)
	if err != nil {
		// 也可能是該網卡只有 IPv6，嘗試 IPv6
		ip6, err6 := FetchInterfaceIP(mockLxcName, "", true)
		if err6 != nil {
			t.Fatalf("無法從帶有 @ 標記的介面 %s 獲取 IP (IPv4 錯誤: %v, IPv6 錯誤: %v)", mockLxcName, err, err6)
		}
		if ip6 == "" {
			t.Errorf("獲得空的 IPv6 地址")
		}
	} else if ip == "" {
		t.Errorf("獲得空的 IPv4 地址")
	}
}

func TestFetchExternalIP_InterfaceBinding(t *testing.T) {
	// 驗證帶有 interface 參數的 FetchExternalIP 調用不會出錯
	defaults := []string{"https://api.ipify.org"}
	_, _ = FetchExternalIP("", false, defaults, "test-host", "lo")
}
