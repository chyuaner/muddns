package web

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"

	"muddns/config"
	"muddns/ipfetcher"
	"muddns/logger"
	"muddns/provider"
)

type Server struct {
	cfg        *config.Config
	configPath string
	tmpl       *template.Template
}

type PageData struct {
	ActiveTab      string
	Hosts          []config.Host
	Providers      map[string]config.Provider
	EditHost       *config.Host
	EditProvider   *config.Provider
	EditProviderID string
	Logs           []logger.LogEntry
	WebAuth        config.WebAuth
	Message        string
	Error          string
}

func NewServer(cfg *config.Config, configPath string, tmplFS fs.FS) (*Server, error) {
	tmpl, err := template.ParseFS(tmplFS, "web/templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	return &Server{
		cfg:        cfg,
		configPath: configPath,
		tmpl:       tmpl,
	}, nil
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.requireAuth(s.handleDashboard))
	mux.HandleFunc("/hosts/edit", s.requireAuth(s.handleHostEdit))
	mux.HandleFunc("/hosts/save", s.requireAuth(s.handleHostSave))
	mux.HandleFunc("/hosts/batch", s.requireAuth(s.handleHostBatch))
	mux.HandleFunc("/providers", s.requireAuth(s.handleProviders))
	mux.HandleFunc("/providers/edit", s.requireAuth(s.handleProviderEdit))
	mux.HandleFunc("/providers/save", s.requireAuth(s.handleProviderSave))
	mux.HandleFunc("/providers/delete", s.requireAuth(s.handleProviderDelete))
	mux.HandleFunc("/logs", s.requireAuth(s.handleLogs))
	mux.HandleFunc("/settings", s.requireAuth(s.handleSettings))
	mux.HandleFunc("/api/preview", s.requireAuth(s.handlePreview))
	mux.HandleFunc("/logout", s.handleLogout)
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.Settings.WebAuth.Enabled {
			next(w, r)
			return
		}

		user, pass, ok := r.BasicAuth()
		if !ok || user != s.cfg.Settings.WebAuth.Username || !s.cfg.Settings.WebAuth.VerifyPassword(pass) {
			w.Header().Set("WWW-Authenticate", `Basic realm="muddns"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	data := PageData{
		ActiveTab: "dashboard",
		Hosts:     s.cfg.Hosts,
		Providers: s.cfg.Providers,
	}
	s.tmpl.ExecuteTemplate(w, "dashboard.html", data)
}

func (s *Server) handleHostEdit(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	var host *config.Host

	if id != "" {
		for _, h := range s.cfg.Hosts {
			if h.ID == id {
				hostCopy := h
				host = &hostCopy
				break
			}
		}
	} else {
		host = &config.Host{
			ID:      fmt.Sprintf("host-%d", len(s.cfg.Hosts)+1),
			Enabled: true,
			IPv4:    config.IPConfig{Enabled: true, Mode: "external_api"},
			IPv6:    config.IPConfig{Enabled: true, Mode: "prefix_stitching", Suffix: "::1"},
		}
	}

	data := PageData{
		ActiveTab: "dashboard",
		Hosts:     s.cfg.Hosts,
		Providers: s.cfg.Providers,
		EditHost:  host,
	}
	s.tmpl.ExecuteTemplate(w, "dashboard.html", data)
}

func (s *Server) handleHostSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	id := r.FormValue("id")
	if id == "" {
		id = fmt.Sprintf("host-%d", len(s.cfg.Hosts)+1)
	}

	domainsRaw := r.FormValue("domains")
	domainLines := strings.Split(domainsRaw, "\n")
	var cleanDomains []string
	for _, d := range domainLines {
		d = strings.TrimSpace(d)
		if d != "" {
			cleanDomains = append(cleanDomains, d)
		}
	}

	ipv4Val := r.FormValue("ipv4_val")
	ipv6Val := r.FormValue("ipv6_val")

	ipv4Cfg := config.IPConfig{
		Enabled:   r.FormValue("ipv4_enabled") == "true",
		Mode:      r.FormValue("ipv4_mode"),
		Interface: r.FormValue("ipv4_interface"),
	}
	switch ipv4Cfg.Mode {
	case "external_api":
		ipv4Cfg.URL = ipv4Val
	case "interface":
		ipv4Cfg.Match = ipv4Val
	case "base_offset":
		ipv4Cfg.Offset = ipv4Val
	case "arp_mac":
		ipv4Cfg.MAC = ipv4Val
	}

	ipv6Cfg := config.IPConfig{
		Enabled:   r.FormValue("ipv6_enabled") == "true",
		Mode:      r.FormValue("ipv6_mode"),
		Interface: r.FormValue("ipv6_interface"),
	}
	switch ipv6Cfg.Mode {
	case "external_api":
		ipv6Cfg.URL = ipv6Val
	case "interface":
		ipv6Cfg.Match = ipv6Val
	case "prefix_stitching":
		ipv6Cfg.Suffix = ipv6Val
	case "eui64_mac":
		ipv6Cfg.MAC = ipv6Val
	}

	// Calculate current preview IPs
	if ipv4Cfg.Enabled {
		ip, err := ipfetcher.ResolveIP(ipv4Cfg, false, s.cfg.Defaults.IPv4APIs, id)
		if err == nil {
			ipv4Cfg.LastIP = ip
		}
	}
	if ipv6Cfg.Enabled {
		ip, err := ipfetcher.ResolveIP(ipv6Cfg, true, s.cfg.Defaults.IPv6APIs, id)
		if err == nil {
			ipv6Cfg.LastIP = ip
		}
	}

	host := config.Host{
		ID:       id,
		Name:     r.FormValue("name"),
		Enabled:  r.FormValue("enabled") == "true",
		Provider: r.FormValue("provider"),
		Domains:  cleanDomains,
		Proxied:  r.FormValue("proxied") == "true",
		IPv4:     ipv4Cfg,
		IPv6:     ipv6Cfg,
	}

	found := false
	for i, h := range s.cfg.Hosts {
		if h.ID == id {
			s.cfg.Hosts[i] = host
			found = true
			break
		}
	}
	if !found {
		s.cfg.Hosts = append(s.cfg.Hosts, host)
	}

	s.cfg.Save(s.configPath)
	logger.Log(logger.INFO, id, "Host %s (%s) saved", host.Name, host.ID)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleHostBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	action := r.FormValue("action")
	selectedIDs := r.Form["host_ids"]

	if len(selectedIDs) == 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	idMap := make(map[string]bool)
	for _, id := range selectedIDs {
		idMap[id] = true
	}

	switch action {
	case "enable":
		for i := range s.cfg.Hosts {
			if idMap[s.cfg.Hosts[i].ID] {
				s.cfg.Hosts[i].Enabled = true
			}
		}
		s.cfg.Save(s.configPath)

	case "disable":
		for i := range s.cfg.Hosts {
			if idMap[s.cfg.Hosts[i].ID] {
				s.cfg.Hosts[i].Enabled = false
			}
		}
		s.cfg.Save(s.configPath)

	case "delete":
		var newHosts []config.Host
		for _, h := range s.cfg.Hosts {
			if !idMap[h.ID] {
				newHosts = append(newHosts, h)
			}
		}
		s.cfg.Hosts = newHosts
		s.cfg.Save(s.configPath)

	case "sync":
		ctx := context.Background()
		for _, h := range s.cfg.Hosts {
			if idMap[h.ID] && h.Enabled {
				s.syncHost(ctx, h)
			}
		}
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) syncHost(ctx context.Context, h config.Host) {
	pCfg, exists := s.cfg.Providers[h.Provider]
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

	// Resolve IPv4
	if h.IPv4.Enabled {
		ip4, err := ipfetcher.ResolveIP(h.IPv4, false, s.cfg.Defaults.IPv4APIs, h.ID)
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

	// Resolve IPv6
	if h.IPv6.Enabled {
		ip6, err := ipfetcher.ResolveIP(h.IPv6, true, s.cfg.Defaults.IPv6APIs, h.ID)
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

func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	data := PageData{
		ActiveTab: "providers",
		Providers: s.cfg.Providers,
	}
	s.tmpl.ExecuteTemplate(w, "providers.html", data)
}

func (s *Server) handleProviderEdit(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	var p *config.Provider

	if id != "" {
		if val, ok := s.cfg.Providers[id]; ok {
			pCopy := val
			p = &pCopy
		}
	} else {
		p = &config.Provider{
			Provider: "cloudflare",
		}
	}

	data := PageData{
		ActiveTab:      "providers",
		Providers:      s.cfg.Providers,
		EditProvider:   p,
		EditProviderID: id,
	}
	s.tmpl.ExecuteTemplate(w, "providers.html", data)
}

func (s *Server) handleProviderSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/providers", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	id := strings.TrimSpace(r.FormValue("id"))
	oldID := strings.TrimSpace(r.FormValue("old_id"))
	pType := r.FormValue("provider")

	if id == "" {
		id = fmt.Sprintf("provider-%d", len(s.cfg.Providers)+1)
	}

	if s.cfg.Providers == nil {
		s.cfg.Providers = make(map[string]config.Provider)
	}

	if oldID != "" && oldID != id {
		delete(s.cfg.Providers, oldID)
	}

	p := config.Provider{
		Provider: pType,
	}

	switch pType {
	case "cloudflare":
		p.APIToken = strings.TrimSpace(r.FormValue("api_token"))
		p.ZoneID = strings.TrimSpace(r.FormValue("zone_id"))
	case "namecheap":
		p.Password = strings.TrimSpace(r.FormValue("password"))
	case "custom_http":
		p.Method = r.FormValue("method")
		p.URL = strings.TrimSpace(r.FormValue("url"))
		p.Body = r.FormValue("body")
		p.ExpectedBodyRegex = strings.TrimSpace(r.FormValue("expected_body_regex"))
	}

	s.cfg.Providers[id] = p
	s.cfg.Save(s.configPath)
	logger.Log(logger.INFO, "", "Provider %s (%s) saved", id, pType)

	http.Redirect(w, r, "/providers", http.StatusSeeOther)
}

func (s *Server) handleProviderDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/providers", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	id := r.FormValue("id")
	if id != "" {
		delete(s.cfg.Providers, id)
		s.cfg.Save(s.configPath)
		logger.Log(logger.INFO, "", "Provider %s deleted", id)
	}

	http.Redirect(w, r, "/providers", http.StatusSeeOther)
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	data := PageData{
		ActiveTab: "settings",
		WebAuth:   s.cfg.Settings.WebAuth,
	}

	if r.Method == "POST" {
		r.ParseForm()
		currentPassword := r.FormValue("current_password")
		newUsername := strings.TrimSpace(r.FormValue("new_username"))
		newPassword := r.FormValue("new_password")
		confirmPassword := r.FormValue("confirm_password")

		if !s.cfg.Settings.WebAuth.VerifyPassword(currentPassword) {
			data.Error = "目前密碼不正確！"
			s.tmpl.ExecuteTemplate(w, "settings.html", data)
			return
		}

		if newPassword == "" {
			data.Error = "新密碼不能為空！"
			s.tmpl.ExecuteTemplate(w, "settings.html", data)
			return
		}

		if newPassword != confirmPassword {
			data.Error = "兩次輸入的新密碼不一致！"
			s.tmpl.ExecuteTemplate(w, "settings.html", data)
			return
		}

		newHash, err := config.HashPassword(newPassword)
		if err != nil {
			data.Error = fmt.Sprintf("密碼加密失敗: %v", err)
			s.tmpl.ExecuteTemplate(w, "settings.html", data)
			return
		}

		if newUsername != "" {
			s.cfg.Settings.WebAuth.Username = newUsername
		}
		s.cfg.Settings.WebAuth.PasswordHash = newHash

		if err := s.cfg.Save(s.configPath); err != nil {
			data.Error = fmt.Sprintf("儲存設定檔失敗: %v", err)
			s.tmpl.ExecuteTemplate(w, "settings.html", data)
			return
		}

		logger.Log(logger.SUCCESS, "", "管理員帳號密碼已成功更新！")
		data.WebAuth = s.cfg.Settings.WebAuth
		data.Message = "管理員帳號與密碼已成功更新！下次登入請使用新密碼。"
	}

	s.tmpl.ExecuteTemplate(w, "settings.html", data)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	hostID := r.URL.Query().Get("host_id")
	level := r.URL.Query().Get("level")

	logs := logger.GlobalRingBuffer.GetLogs(hostID, level)

	data := PageData{
		ActiveTab: "logs",
		Logs:      logs,
	}
	s.tmpl.ExecuteTemplate(w, "logs.html", data)
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	mode := r.FormValue("ipv4_mode")
	isIPv6 := false
	if mode == "" {
		mode = r.FormValue("ipv6_mode")
		isIPv6 = true
	}

	ipCfg := config.IPConfig{
		Enabled:   true,
		Mode:      mode,
		Interface: r.FormValue("ipv4_interface"),
	}
	if isIPv6 {
		ipCfg.Interface = r.FormValue("ipv6_interface")
		val := r.FormValue("ipv6_val")
		switch mode {
		case "prefix_stitching":
			ipCfg.Suffix = val
		case "eui64_mac":
			ipCfg.MAC = val
		}
	} else {
		val := r.FormValue("ipv4_val")
		switch mode {
		case "interface":
			ipCfg.Match = val
		case "base_offset":
			ipCfg.Offset = val
		case "arp_mac":
			ipCfg.MAC = val
		}
	}

	defaults := s.cfg.Defaults.IPv4APIs
	if isIPv6 {
		defaults = s.cfg.Defaults.IPv6APIs
	}

	ip, err := ipfetcher.ResolveIP(ipCfg, isIPv6, defaults, "preview")
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, "🔍 即時預覽: <span style='color: var(--danger);'>計算失敗 (%v)</span>", err)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, "🔍 即時預覽: <strong style='color: var(--success);'>%s</strong>", ip)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", `Basic realm="muddns"`)
	http.Error(w, "Logged out", http.StatusUnauthorized)
}
