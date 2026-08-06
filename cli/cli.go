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
	"os/signal"
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

	_ = cmdFlags.Parse(os.Args[2:])

	switch command {
	case "serve":
		runServe(*configPath, *debug)
	case "sync":
		runSync(*configPath, *targetHost, *debug)
	case "status":
		runStatus(*configPath, *debug)
	case "version":
		fmt.Printf("muddns 版本: %s\n", version)
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
	fmt.Println("  sync       執行單次即時 DNS 同步更新 (適合搭配 cron 執行)")
	fmt.Println("  status     計算並列出所有主機當前的 IP (乾跑測試，不實際修改 DNS)")
	fmt.Println("  version    顯示 muddns 版本號")
	fmt.Println("\n常用選項:")
	fmt.Println("  -c <path>  指定設定檔路徑 (預設: config.yaml)")
	fmt.Println("  -h <host>  指定僅更新特定主機 ID (適用於 sync 命令)")
	fmt.Println("  -debug     開啟詳細除錯日誌模式")
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

	// 啟動時立即執行首次同步
	logger.Log(logger.INFO, "", "啟動時執行首次 DNS 同步檢查...")
	syncAllHosts(ctx, cfg, configPath)

	for {
		select {
		case <-ticker.C:
			// 重新載入最新設定檔
			if reloadedCfg, err := config.LoadConfig(configPath); err == nil {
				cfg = reloadedCfg
			}
			logger.Log(logger.INFO, "", "觸發定時輪詢 DNS 同步...")
			syncAllHosts(ctx, cfg, configPath)

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
func runSync(configPath string, targetHost string, debug bool) {
	logger.VerboseDebug = debug

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("[ERROR] 載入設定檔失敗: %v\n", err)
		os.Exit(1)
	}

	setupSystemCAs(cfg.Settings.CustomCAFile, cfg.Settings.InsecureSkipVerify)

	ctx := context.Background()
	for _, h := range cfg.Hosts {
		if !h.Enabled {
			continue
		}
		if targetHost != "" && h.ID != targetHost {
			continue
		}
		syncHost(ctx, cfg, h)
	}

	cfg.Save(configPath)
	fmt.Println("[SUCCESS] 單次同步完成。")
}

// runStatus 執行乾跑 (Dry Run) 測試，列出算得 IP 而不修改 DNS
func runStatus(configPath string, debug bool) {
	logger.VerboseDebug = debug

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("[ERROR] 載入設定檔失敗: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n[+] 載入設定檔: %s\n", configPath)
	fmt.Println("==========================================================================================")
	fmt.Printf("%-12s %-18s %-25s %-25s\n", "Host ID", "Name", "IPv4 (Calculated)", "IPv6 (Calculated)")
	fmt.Println("==========================================================================================")

	for _, h := range cfg.Hosts {
		ip4 := "-"
		if h.IPv4.Enabled {
			res, err := ipfetcher.ResolveIP(h.IPv4, false, cfg.Defaults.IPv4APIs, h.ID)
			if err == nil {
				ip4 = res + " (" + h.IPv4.Mode + ")"
			} else {
				ip4 = "Error: " + err.Error()
			}
		}

		ip6 := "-"
		if h.IPv6.Enabled {
			res, err := ipfetcher.ResolveIP(h.IPv6, true, cfg.Defaults.IPv6APIs, h.ID)
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
func syncAllHosts(ctx context.Context, cfg *config.Config, configPath string) {
	for _, h := range cfg.Hosts {
		if !h.Enabled {
			continue
		}
		syncHost(ctx, cfg, h)
	}
	cfg.Save(configPath)
}

// syncHost 執行單一主機的 IP 檢測與 DNS 紀錄更新
func syncHost(ctx context.Context, cfg *config.Config, h config.Host) {
	pConfig, ok := cfg.Providers[h.Provider]
	if !ok {
		logger.Log(logger.ERROR, h.ID, "找不到綁定的 DNS Provider: %s", h.Provider)
		return
	}

	p, err := provider.NewProvider(pConfig)
	if err != nil {
		logger.Log(logger.ERROR, h.ID, "初始化 Provider %s 失敗: %v", h.Provider, err)
		return
	}

	// 處理 IPv4 更新
	if h.IPv4.Enabled {
		ip4, err := ipfetcher.ResolveIP(h.IPv4, false, cfg.Defaults.IPv4APIs, h.ID)
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
	if h.IPv6.Enabled {
		ip6, err := ipfetcher.ResolveIP(h.IPv6, true, cfg.Defaults.IPv6APIs, h.ID)
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
