package main

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
	"muddns/ipfetcher"
	"muddns/logger"
	"muddns/provider"
	"muddns/web"
)

func main() {
	if len(os.Args) < 2 {
		runServe("config.yaml", false)
		return
	}

	subCmd := os.Args[1]

	switch subCmd {
	case "serve":
		serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
		configPath := serveCmd.String("c", "config.yaml", "Path to config file")
		debug := serveCmd.Bool("debug", false, "Enable verbose debug logging")
		serveCmd.Parse(os.Args[2:])
		runServe(*configPath, *debug)

	case "sync":
		syncCmd := flag.NewFlagSet("sync", flag.ExitOnError)
		configPath := syncCmd.String("c", "config.yaml", "Path to config file")
		targetHost := syncCmd.String("host", "", "Specific host ID to sync")
		debug := syncCmd.Bool("debug", false, "Enable verbose debug logging")
		syncCmd.Parse(os.Args[2:])
		runSync(*configPath, *targetHost, *debug)

	case "status":
		statusCmd := flag.NewFlagSet("status", flag.ExitOnError)
		configPath := statusCmd.String("c", "config.yaml", "Path to config file")
		debug := statusCmd.Bool("debug", false, "Enable verbose debug logging")
		statusCmd.Parse(os.Args[2:])
		runStatus(*configPath, *debug)

	default:
		// Default to serve if first argument is a flag like -c or --debug
		if os.Args[1][0] == '-' {
			serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
			configPath := serveCmd.String("c", "config.yaml", "Path to config file")
			debug := serveCmd.Bool("debug", false, "Enable verbose debug logging")
			serveCmd.Parse(os.Args[1:])
			runServe(*configPath, *debug)
			return
		}

		fmt.Println("Usage: muddns [serve|sync|status] [-c config.yaml] [--debug]")
		os.Exit(1)
	}
}

func setupSystemCAs(customCAPath string, insecureSkipVerify bool) {
	if customCAPath == "" && !insecureSkipVerify {
		return
	}

	rootCAs, _ := x509.SystemCertPool()
	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}

	if customCAPath != "" {
		caData, err := os.ReadFile(customCAPath)
		if err == nil {
			rootCAs.AppendCertsFromPEM(caData)
		}
	}

	http.DefaultTransport = &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:            rootCAs,
			InsecureSkipVerify: insecureSkipVerify,
		},
	}
}

func runServe(configPath string, debug bool) {
	logger.VerboseDebug = debug

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		logger.Log(logger.ERROR, "", "Failed to load config from %s: %v", configPath, err)
		os.Exit(1)
	}

	if cfg.Settings.LogFile != "" {
		logger.SetLogFile(cfg.Settings.LogFile)
	}

	setupSystemCAs(cfg.Settings.CustomCAFile, cfg.Settings.InsecureSkipVerify)

	srv, err := web.NewServer(cfg, configPath, templateFS)
	if err != nil {
		logger.Log(logger.ERROR, "", "Failed to init web server: %v", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	httpServer := &http.Server{
		Addr:    cfg.Settings.Listen,
		Handler: mux,
	}

	logger.Log(logger.INFO, "", "muddns web server listening on %s", cfg.Settings.Listen)

	// Start background ticker loop
	ticker := time.NewTicker(time.Duration(cfg.Settings.IntervalSeconds) * time.Second)
	defer ticker.Stop()

	go func() {
		// Run immediate sync at startup
		performAllSync(cfg)

		for range ticker.C {
			performAllSync(cfg)
		}
	}()

	// Graceful shutdown listener
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log(logger.ERROR, "", "HTTP server error: %v", err)
		}
	}()

	<-stopChan
	logger.Log(logger.INFO, "", "Shutting down muddns server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)
}

func runSync(configPath string, targetHost string, debug bool) {
	logger.VerboseDebug = debug

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("[ERROR] Failed to load config: %v\n", err)
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
	fmt.Println("[SUCCESS] One-shot sync completed.")
}

func runStatus(configPath string, debug bool) {
	logger.VerboseDebug = debug

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("[ERROR] Failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n[+] Loading config from %s\n", configPath)
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
	fmt.Println("[i] Status dry-run complete. No DNS records were modified.\n")
}

func performAllSync(cfg *config.Config) {
	ctx := context.Background()
	for _, h := range cfg.Hosts {
		if h.Enabled {
			syncHost(ctx, cfg, h)
		}
	}
}

func syncHost(ctx context.Context, cfg *config.Config, h config.Host) {
	pCfg, exists := cfg.Providers[h.Provider]
	if !exists {
		logger.Log(logger.ERROR, h.ID, "Provider %s not found for host %s", h.Provider, h.Name)
		return
	}

	var drv provider.Provider
	var err error
	switch pCfg.Provider {
	case "cloudflare":
		drv, err = provider.NewCloudflareProvider(h.Provider, pCfg.APIToken, pCfg.ZoneID)
	case "namecheap":
		drv = provider.NewNamecheapProvider(h.Provider, pCfg.Password)
	case "custom_http":
		drv = provider.NewCustomHTTPProvider(h.Provider, provider.CustomHTTPConfig{
			Method:            pCfg.Method,
			URL:               pCfg.URL,
			Headers:           pCfg.Headers,
			Body:              pCfg.Body,
			ExpectedStatus:    pCfg.ExpectedStatus,
			ExpectedBodyRegex: pCfg.ExpectedBodyRegex,
		})
	}

	if err != nil {
		logger.Log(logger.ERROR, h.ID, "Failed to init provider %s: %v", h.Provider, err)
		return
	}

	if h.IPv4.Enabled {
		ip4, err := ipfetcher.ResolveIP(h.IPv4, false, cfg.Defaults.IPv4APIs, h.ID)
		if err != nil {
			logger.Log(logger.ERROR, h.ID, "Failed to resolve IPv4: %v", err)
		} else {
			for _, domain := range h.Domains {
				sub, root := provider.ExtractRootDomain(domain)
				rec := provider.TargetRecord{
					HostID:     h.ID,
					Domain:     domain,
					Subdomain:  sub,
					RootDomain: root,
					IP:         ip4,
					Type:       provider.RecordTypeA,
					Proxied:    h.Proxied,
				}
				if err := drv.UpdateRecord(ctx, rec); err != nil {
					logger.Log(logger.ERROR, h.ID, "Failed to update IPv4 A record for %s: %v", domain, err)
				}
			}
		}
	}

	if h.IPv6.Enabled {
		ip6, err := ipfetcher.ResolveIP(h.IPv6, true, cfg.Defaults.IPv6APIs, h.ID)
		if err != nil {
			logger.Log(logger.ERROR, h.ID, "Failed to resolve IPv6: %v", err)
		} else {
			for _, domain := range h.Domains {
				sub, root := provider.ExtractRootDomain(domain)
				rec := provider.TargetRecord{
					HostID:     h.ID,
					Domain:     domain,
					Subdomain:  sub,
					RootDomain: root,
					IP:         ip6,
					Type:       provider.RecordTypeAAAA,
					Proxied:    h.Proxied,
				}
				if err := drv.UpdateRecord(ctx, rec); err != nil {
					logger.Log(logger.ERROR, h.ID, "Failed to update IPv6 AAAA record for %s: %v", domain, err)
				}
			}
		}
	}
}
