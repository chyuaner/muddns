// Package cli 負責命令列介面 (CLI) 的指令解析與核心工作流分配 (serve, sync, status)。
package cli

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"muddns/config"
	"muddns/lib/ipfetcher"
	"muddns/lib/logger"
	"muddns/lib/provider"
	"muddns/web"
)

// Run 為 CLI 的主要派發入口，解析 flag 並執行對應子命令
func Run(version string) {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	// 建立帶預設值的通用 FlagSet
	cmdFlags := flag.NewFlagSet(command, flag.ExitOnError)
	configPath := cmdFlags.String("c", "config.yaml", "指定設定檔路徑 (預設: config.yaml)")
	targetHost := cmdFlags.String("h", "", "指定僅同步特定 Host ID (適用於 sync 命令)")
	debug := cmdFlags.Bool("debug", false, "是否開啟 Verbose Debug 日誌輸出")

	var filterIface string
	cmdFlags.StringVar(&filterIface, "interface", "", "指定僅更新綁定特定網卡介面的主機 (例: eth0, pppoe0) (簡寫: -i)")
	cmdFlags.StringVar(&filterIface, "i", "", "指定僅更新綁定特定網卡介面的主機 (簡寫)")

	_ = cmdFlags.Parse(os.Args[2:])

	switch command {
	case "serve":
		runServe(*configPath, *debug)
	case "sync":
		runSync(*configPath, *targetHost, filterIface, *debug)
	case "status":
		runStatus(*configPath, filterIface, *debug)
	case "version":
		fmt.Printf("muddns 版本: %s\n", version)
	case "install":
		runInstallService(*configPath)
	case "uninstall":
		runUninstallService()
	case "service":
		subCmd := ""
		if len(os.Args) >= 3 {
			subCmd = os.Args[2]
		}
		switch subCmd {
		case "install":
			runInstallService(*configPath)
		case "uninstall":
			runUninstallService()
		case "start", "stop", "restart", "status":
			runControlService(subCmd)
		default:
			fmt.Println("[!] 未知的 service 子指令。可用的服務指令: install, uninstall, start, stop, restart, status")
			fmt.Println("    範例: sudo ./muddns service install -c /etc/muddns/config.yaml")
			os.Exit(1)
		}
	default:
		fmt.Printf("[ERROR] 未知的命令: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

// printUsage 列印 CLI 命令說明手冊
func printUsage() {
	fmt.Println("muddns - 輕量級多主機 DDNS 動態域名同步工具")
	fmt.Println("\n用法:")
	fmt.Println("  muddns <command> [options]")
	fmt.Println("\n可用子命令:")
	fmt.Println("  serve      啟動背景輪詢更新服務並開啟 Web 管理介面")
	fmt.Println("  sync       執行單次即時 DNS 同步更新 (適合搭配 cron 或 OPNsense WAN 重新連線腳本觸發)")
	fmt.Println("  status     計算並列出所有主機當前的 IP (乾跑測試，不實際修改 DNS)")
	fmt.Println("  install    將 muddns 註冊為 Linux Systemd 常駐服務 (同 service install)")
	fmt.Println("  uninstall  停止並移除 muddns Linux Systemd 常駐服務 (同 service uninstall)")
	fmt.Println("  service    常駐服務管理 (install, uninstall, start, stop, restart, status)")
	fmt.Println("  version    顯示 muddns 版本號")
	fmt.Println("\n常用選項:")
	fmt.Println("  -c <path>         指定設定檔路徑 (預設: config.yaml)")
	fmt.Println("  -i, --interface   僅同步綁定指定網卡介面的主機 (例: -i pppoe0，未指定時更新全部)")
	fmt.Println("  -h <host>         指定僅更新特定主機 ID (適用於 sync 命令)")
	fmt.Println("  -debug            開啟詳細除錯日誌模式")
}

// runServe 啟動背景 Web UI 伺服器與定時輪詢 Task
func runServe(configPath string, debug bool) {
	logger.VerboseDebug = debug

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("[ERROR] 載入設定檔失敗: %v\n", err)
		os.Exit(1)
	}

	setupSystemCAs(cfg.Settings.CustomCAFile, cfg.Settings.InsecureSkipVerify)

	// 初始化 Web 伺服器
	webFS := web.GetWebFS()
	srv, err := web.NewServer(cfg, configPath, webFS)
	if err != nil {
		fmt.Printf("[ERROR] 初始化 Web Server 失敗: %v\n", err)
		os.Exit(1)
	}

	// 啟動 Web 服務執行緒
	go func() {
		if err := srv.Start(); err != nil {
			logger.Log(logger.ERROR, "", "Web 伺服器例外終止: %v", err)
		}
	}()

	// 啟動背景定時輪詢 Ticker
	interval := time.Duration(cfg.Settings.IntervalSeconds) * time.Second
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 捕捉 Ctrl+C / SIGTERM 優雅退出訊號
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 啟動時執行首次同步
	logger.Log(logger.INFO, "", "啟動時執行首次 DNS 同步檢查...")
	syncAllHosts(ctx, cfg, configPath, "")

	for {
		select {
		case <-ticker.C:
			// 重新載入最新設定檔
			if reloadedCfg, err := config.LoadConfig(configPath); err == nil {
				cfg = reloadedCfg
			}
			logger.Log(logger.INFO, "", "觸發定時輪詢 DNS 同步...")
			syncAllHosts(ctx, cfg, configPath, "")

		case sig := <-sigChan:
			logger.Log(logger.INFO, "", "接收到關閉訊號 (%v)，準備安全關閉服務器...", sig)
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = srv.Shutdown(shutdownCtx)
			logger.Log(logger.INFO, "", "muddns 服務已安全終止。")
			return
		}
	}
}

// runSync 執行單次即時 DNS 更新
func runSync(configPath string, targetHost string, filterIface string, debug bool) {
	logger.VerboseDebug = debug

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("[ERROR] 載入設定檔失敗: %v\n", err)
		os.Exit(1)
	}

	setupSystemCAs(cfg.Settings.CustomCAFile, cfg.Settings.InsecureSkipVerify)

	if filterIface != "" {
		logger.Log(logger.INFO, "", "指定網卡介面篩選: %s (僅同步綁定此介面的主機)", filterIface)
	}

	ctx := context.Background()
	syncedCount := 0
	for _, h := range cfg.Hosts {
		if !h.Enabled {
			continue
		}
		if targetHost != "" && h.ID != targetHost {
			continue
		}
		if syncHost(ctx, cfg, h, filterIface) {
			syncedCount++
		}
	}

	cfg.Save(configPath)
	fmt.Printf("[SUCCESS] 單次同步完成。共處理 %d 台主機。\n", syncedCount)
}

// runStatus 執行乾跑 (Dry Run) 測試，列出算得 IP 而不修改 DNS
func runStatus(configPath string, filterIface string, debug bool) {
	logger.VerboseDebug = debug

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("[ERROR] 載入設定檔失敗: %v\n", err)
		os.Exit(1)
	}

	if filterIface != "" {
		fmt.Printf("[+] 指定網卡介面篩選: %s\n", filterIface)
	}

	fmt.Printf("\n[+] 載入設定檔: %s\n", configPath)
	fmt.Println("==========================================================================================")
	fmt.Printf("%-12s %-18s %-25s %-25s\n", "Host ID", "Name", "IPv4 (Calculated)", "IPv6 (Calculated)")
	fmt.Println("==========================================================================================")

	for _, h := range cfg.Hosts {
		// 檢查介面篩選
		v4Iface := h.IPv4.Interface
		if v4Iface == "" {
			v4Iface = cfg.Settings.DefaultIPv4Interface
		}
		v6Iface := h.IPv6.Interface
		if v6Iface == "" {
			v6Iface = cfg.Settings.DefaultIPv6Interface
		}

		v4Matches := h.IPv4.Enabled && (filterIface == "" || v4Iface == filterIface)
		v6Matches := h.IPv6.Enabled && (filterIface == "" || v6Iface == filterIface)

		if filterIface != "" && !v4Matches && !v6Matches {
			continue
		}

		ip4 := "-"
		if v4Matches {
			res, err := ipfetcher.ResolveIP(h.IPv4, false, cfg.Defaults.IPv4APIs, h.ID, cfg.Settings.DefaultIPv4Interface)
			if err == nil {
				ip4 = res + " (" + h.IPv4.Mode + ")"
			} else {
				ip4 = "Error: " + err.Error()
			}
		}

		ip6 := "-"
		if v6Matches {
			res, err := ipfetcher.ResolveIP(h.IPv6, true, cfg.Defaults.IPv6APIs, h.ID, cfg.Settings.DefaultIPv6Interface)
			if err == nil {
				ip6 = res + " (" + h.IPv6.Mode + ")"
			} else {
				ip6 = "Error: " + err.Error()
			}
		}

		fmt.Printf("%-12s %-18s %-25s %-25s\n", h.ID, h.Name, ip4, ip6)
	}
	fmt.Println("==========================================================================================")
	fmt.Println("[i] 狀態乾跑測試完畢。未發送任何 DNS 修改請求。")
}

// syncAllHosts 遍歷同步所有啟用的主機
func syncAllHosts(ctx context.Context, cfg *config.Config, configPath string, filterIface string) {
	for _, h := range cfg.Hosts {
		if !h.Enabled {
			continue
		}
		syncHost(ctx, cfg, h, filterIface)
	}
	cfg.Save(configPath)
}

// syncHost 執行單一主機的 IP 檢測與 DNS 紀錄更新，回傳 bool 代表是否有執行同步
func syncHost(ctx context.Context, cfg *config.Config, h config.Host, filterIface string) bool {
	v4Iface := h.IPv4.Interface
	if v4Iface == "" {
		v4Iface = cfg.Settings.DefaultIPv4Interface
	}
	v6Iface := h.IPv6.Interface
	if v6Iface == "" {
		v6Iface = cfg.Settings.DefaultIPv6Interface
	}

	v4Matches := h.IPv4.Enabled && (filterIface == "" || v4Iface == filterIface)
	v6Matches := h.IPv6.Enabled && (filterIface == "" || v6Iface == filterIface)

	if filterIface != "" && !v4Matches && !v6Matches {
		return false
	}

	pConfig, ok := cfg.Providers[h.Provider]
	if !ok {
		logger.Log(logger.ERROR, h.ID, "找不到綁定的 DNS Provider: %s", h.Provider)
		return false
	}

	p, err := provider.NewProvider(pConfig)
	if err != nil {
		logger.Log(logger.ERROR, h.ID, "初始化 Provider %s 失敗: %v", h.Provider, err)
		return false
	}

	// 處理 IPv4 更新
	if v4Matches {
		ip4, err := ipfetcher.ResolveIP(h.IPv4, false, cfg.Defaults.IPv4APIs, h.ID, cfg.Settings.DefaultIPv4Interface)
		if err != nil {
			logger.Log(logger.ERROR, h.ID, "計算 IPv4 失敗: %v", err)
		} else {
			// 快取上次成功 IP
			for i, hostRef := range cfg.Hosts {
				if hostRef.ID == h.ID {
					cfg.Hosts[i].IPv4.LastIP = ip4
					break
				}
			}
			for _, domain := range h.Domains {
				if err := p.UpdateRecord(ctx, domain, provider.RecordA, ip4, h.Proxied); err != nil {
					logger.Log(logger.ERROR, h.ID, "更新 IPv4 A 紀錄失敗 (%s): %v", domain, err)
				}
			}
		}
	}

	// 處理 IPv6 更新
	if v6Matches {
		ip6, err := ipfetcher.ResolveIP(h.IPv6, true, cfg.Defaults.IPv6APIs, h.ID, cfg.Settings.DefaultIPv6Interface)
		if err != nil {
			logger.Log(logger.ERROR, h.ID, "計算 IPv6 失敗: %v", err)
		} else {
			// 快取上次成功 IP
			for i, hostRef := range cfg.Hosts {
				if hostRef.ID == h.ID {
					cfg.Hosts[i].IPv6.LastIP = ip6
					break
				}
			}
			for _, domain := range h.Domains {
				if err := p.UpdateRecord(ctx, domain, provider.RecordAAAA, ip6, h.Proxied); err != nil {
					logger.Log(logger.ERROR, h.ID, "更新 IPv6 AAAA 紀錄失敗 (%s): %v", domain, err)
				}
			}
		}
	}

	return true
}

// setupSystemCAs 設定系統 TLS 憑證與自訂 CA
func setupSystemCAs(customCA string, insecureSkip bool) {
	if customCA == "" && !insecureSkip {
		return
	}

	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return
	}

	if insecureSkip {
		if tr.TLSClientConfig == nil {
			tr.TLSClientConfig = &tls.Config{}
		}
		tr.TLSClientConfig.InsecureSkipVerify = true
		logger.Log(logger.WARN, "", "已開啟不安全性跳過 TLS 憑證驗證 (insecure_skip_verify)")
		return
	}

	if customCA != "" {
		caCert, err := os.ReadFile(customCA)
		if err != nil {
			logger.Log(logger.ERROR, "", "讀取自訂 CA 憑證失敗: %v", err)
			return
		}
		caCertPool, _ := x509.SystemCertPool()
		if caCertPool == nil {
			caCertPool = x509.NewCertPool()
		}
		caCertPool.AppendCertsFromPEM(caCert)
		if tr.TLSClientConfig == nil {
			tr.TLSClientConfig = &tls.Config{}
		}
		tr.TLSClientConfig.RootCAs = caCertPool
		logger.Log(logger.INFO, "", "已成功載入自訂 CA 憑證: %s", customCA)
	}
}

// isOpenRC 檢查目前作業系統是否採用 OpenRC Init 系統 (例如 Alpine Linux)
func isOpenRC() bool {
	if _, err := os.Stat("/sbin/openrc-run"); err == nil {
		return true
	}
	if _, err := os.Stat("/etc/alpine-release"); err == nil {
		return true
	}
	return false
}

// runInstallService 自動將 muddns 註冊為常駐服務 (支援 Linux Systemd 與 OpenRC)
func runInstallService(configPath string) {
	if runtime.GOOS != "linux" {
		fmt.Println("[!] 目前自動註冊服務功能僅支援 Linux (Systemd / OpenRC)。")
		os.Exit(1)
	}

	if os.Geteuid() != 0 {
		fmt.Println("[!] 錯誤：安裝系統服務需要 root 權限，請加上 sudo 重新執行:")
		fmt.Printf("    sudo %s service install -c %s\n", os.Args[0], configPath)
		os.Exit(1)
	}

	execPath, err := os.Executable()
	if err != nil {
		fmt.Printf("[ERROR] 無法取得 muddns 可執行檔路徑: %v\n", err)
		os.Exit(1)
	}

	absExecPath, err := filepath.Abs(execPath)
	if err != nil {
		fmt.Printf("[ERROR] 無法計算 Executable 絕對路徑: %v\n", err)
		os.Exit(1)
	}

	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		fmt.Printf("[ERROR] 無法計算 設定檔 絕對路徑: %v\n", err)
		os.Exit(1)
	}

	workDir := filepath.Dir(absConfigPath)

	if isOpenRC() {
		// Alpine Linux / OpenRC 初始化流程
		serviceContent := fmt.Sprintf(`#!/sbin/openrc-run

name="muddns"
description="muddns - Multi-host DDNS Service"
command="%s"
command_args="serve -c %s"
command_background="yes"
pidfile="/run/${RC_SVCNAME}.pid"
directory="%s"

depend() {
	need net
	after firewall
}
`, absExecPath, absConfigPath, workDir)

		serviceFilePath := "/etc/init.d/muddns"
		err = os.WriteFile(serviceFilePath, []byte(serviceContent), 0755)
		if err != nil {
			fmt.Printf("[ERROR] 寫入 OpenRC 服務設定檔失敗 (%s): %v\n", serviceFilePath, err)
			os.Exit(1)
		}

		fmt.Println("[+] 成功創建 OpenRC 服務腳本: " + serviceFilePath)

		if err := exec.Command("rc-update", "add", "muddns", "default").Run(); err != nil {
			fmt.Printf("[!] rc-update add 失敗: %v\n", err)
		}
		if err := exec.Command("rc-service", "muddns", "start").Run(); err != nil {
			fmt.Printf("[!] rc-service start 失敗: %v\n", err)
		} else {
			fmt.Println("[+] 成功啟動並開啟 OpenRC 常駐服務: muddns")
		}

		fmt.Println("\n================================================================================")
		fmt.Println("  🎉 muddns OpenRC 常駐服務已安裝並成功啟動！(Alpine Linux)")
		fmt.Println("================================================================================")
		fmt.Printf("  • 執行檔路徑 : %s\n", absExecPath)
		fmt.Printf("  • 設定檔路徑 : %s\n", absConfigPath)
		fmt.Println("  • 查看服務狀態: rc-service muddns status")
		fmt.Println("  • 停止服務    : rc-service muddns stop")
		fmt.Println("  • 卸載常駐服務: sudo ./muddns service uninstall")
		fmt.Println("================================================================\n")
		return
	}

	// Systemd 初始化流程 (Ubuntu / Debian / CentOS / Arch)
	_ = os.MkdirAll("/etc/systemd/system", 0755)

	serviceContent := fmt.Sprintf(`[Unit]
Description=muddns - Multi-host DDNS Service
Documentation=https://github.com/chyuaner/muddns
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=%s
ExecStart=%s serve -c %s
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
`, workDir, absExecPath, absConfigPath)

	serviceFilePath := "/etc/systemd/system/muddns.service"
	err = os.WriteFile(serviceFilePath, []byte(serviceContent), 0644)
	if err != nil {
		fmt.Printf("[ERROR] 寫入服務設定檔失敗 (%s): %v\n", serviceFilePath, err)
		os.Exit(1)
	}

	fmt.Println("[+] 成功創建 Systemd 服務檔案: " + serviceFilePath)

	_ = exec.Command("systemctl", "daemon-reload").Run()
	if err := exec.Command("systemctl", "enable", "muddns.service").Run(); err != nil {
		fmt.Printf("[!] 啟用服務 enable 失敗: %v\n", err)
	}
	if err := exec.Command("systemctl", "start", "muddns.service").Run(); err != nil {
		fmt.Printf("[!] 啟動服務 start 失敗: %v\n", err)
	} else {
		fmt.Println("[+] 成功啟動並開啟常駐服務: muddns.service")
	}

	fmt.Println("\n================================================================================")
	fmt.Println("  🎉 muddns Systemd 常駐服務已安裝並成功啟動！")
	fmt.Println("================================================================================")
	fmt.Printf("  • 執行檔路徑 : %s\n", absExecPath)
	fmt.Printf("  • 設定檔路徑 : %s\n", absConfigPath)
	fmt.Println("  • 查看服務狀態: systemctl status muddns")
	fmt.Println("  • 查看服務日誌: journalctl -u muddns -f")
	fmt.Println("  • 停止服務    : systemctl stop muddns")
	fmt.Println("  • 卸載常駐服務: sudo ./muddns service uninstall")
	fmt.Println("================================================================\n")
}

// runUninstallService 停止、關閉並移除系統服務 (Systemd 或 OpenRC)
func runUninstallService() {
	if runtime.GOOS != "linux" {
		fmt.Println("[!] 目前自動註冊服務功能僅原生支援 Linux Systemd。")
		os.Exit(1)
	}

	if os.Geteuid() != 0 {
		fmt.Println("[!] 錯誤：移除 Systemd 系統服務需要 root 權限，請加上 sudo 執行:")
		fmt.Println("    sudo ./muddns service uninstall")
		os.Exit(1)
	}

	serviceFilePath := "/etc/systemd/system/muddns.service"

	fmt.Println("[i] 正在停止與關閉 muddns.service...")
	_ = exec.Command("systemctl", "stop", "muddns.service").Run()
	_ = exec.Command("systemctl", "disable", "muddns.service").Run()

	if _, err := os.Stat(serviceFilePath); err == nil {
		if err := os.Remove(serviceFilePath); err != nil {
			fmt.Printf("[!] 刪除服務檔 %s 失敗: %v\n", serviceFilePath, err)
		} else {
			fmt.Println("[+] 已成功刪除服務檔案: " + serviceFilePath)
		}
	}

	_ = exec.Command("systemctl", "daemon-reload").Run()
	fmt.Println("[+] 已成功解除安裝 muddns 常駐服務。")
}

// runControlService 管理 Systemd 服務狀態 (start, stop, restart, status)
func runControlService(action string) {
	if runtime.GOOS != "linux" {
		fmt.Println("[!] 僅支援 Linux Systemd。")
		os.Exit(1)
	}
	cmd := exec.Command("systemctl", action, "muddns.service")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Printf("[!] 執行 systemctl %s muddns 失敗: %v\n", action, err)
	}
}
