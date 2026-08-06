package web

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"muddns/config"
	"muddns/lib/ipfetcher"
	"muddns/lib/logger"
	"muddns/lib/provider"

	"gopkg.in/yaml.v3"
)

// renderTemplate 渲染指定名稱的 HTML 範本，若發生錯誤會記錄日誌並回傳 HTTP 500
func (s *Server) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		logger.Log(logger.ERROR, "", "渲染範本 %s 失敗: %v", name, err)
		http.Error(w, fmt.Sprintf("500 Internal Server Error (Template Error: %v)", err), http.StatusInternalServerError)
	}
}

// handleDashboard 處理 Dashboard 主控板首頁請求 (/)
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// 將所有主機依 DNS Provider 進行分類群組化
	grouped := make(map[string][]config.Host)
	for _, h := range s.cfg.Hosts {
		p := h.Provider
		if p == "" {
			p = "default"
		}
		grouped[p] = append(grouped[p], h)
	}

	data := PageData{
		ActiveTab:    "dashboard",
		Hosts:        s.cfg.Hosts,
		GroupedHosts: grouped,
		Providers:    s.cfg.Providers,
	}
	s.renderTemplate(w, "dashboard.html", data)
}

// handleHostEdit 處理單一主機新增或編輯頁面 (/hosts/edit)
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
		// 新增預設範本
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
	s.renderTemplate(w, "dashboard.html", data)
}

// handleHostSave 處理單一主機儲存動作 (/hosts/save)
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
	case "command":
		ipv4Cfg.Command = ipv4Val
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
	case "command":
		ipv6Cfg.Command = ipv6Val
	}

	// 嘗試即時計算快取 IP
	if ipv4Cfg.Enabled {
		if ip, err := ipfetcher.ResolveIP(ipv4Cfg, false, s.cfg.Defaults.IPv4APIs, id); err == nil {
			ipv4Cfg.LastIP = ip
		}
	}
	if ipv6Cfg.Enabled {
		if ip, err := ipfetcher.ResolveIP(ipv6Cfg, true, s.cfg.Defaults.IPv6APIs, id); err == nil {
			ipv6Cfg.LastIP = ip
		}
	}

	newHost := config.Host{
		ID:       id,
		Name:     strings.TrimSpace(r.FormValue("name")),
		Enabled:  r.FormValue("enabled") == "true",
		Provider: r.FormValue("provider"),
		Domains:  cleanDomains,
		Proxied:  r.FormValue("proxied") == "true",
		IPv4:     ipv4Cfg,
		IPv6:     ipv6Cfg,
	}

	// 更新已有主機或追加新主機
	found := false
	for i, h := range s.cfg.Hosts {
		if h.ID == id {
			s.cfg.Hosts[i] = newHost
			found = true
			break
		}
	}
	if !found {
		s.cfg.Hosts = append(s.cfg.Hosts, newHost)
	}

	s.cfg.Save(s.configPath)
	logger.Log(logger.INFO, id, "主機設定 %s 已成功儲存", newHost.Name)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleHostBatch 處理 Dashboard 多選主機之批量操作 (/hosts/batch: 同步, 啟用, 停用, 刪除)
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
	case "sync":
		ctx := context.Background()
		for _, h := range s.cfg.Hosts {
			if idMap[h.ID] && h.Enabled {
				go s.syncSingleHost(ctx, h)
			}
		}
		logger.Log(logger.INFO, "", "已為選取的 %d 台主機觸發手動背景同步", len(selectedIDs))

	case "enable":
		for i, h := range s.cfg.Hosts {
			if idMap[h.ID] {
				s.cfg.Hosts[i].Enabled = true
			}
		}
		s.cfg.Save(s.configPath)
		logger.Log(logger.INFO, "", "已批量啟用選取的 %d 台主機", len(selectedIDs))

	case "disable":
		for i, h := range s.cfg.Hosts {
			if idMap[h.ID] {
				s.cfg.Hosts[i].Enabled = false
			}
		}
		s.cfg.Save(s.configPath)
		logger.Log(logger.INFO, "", "已批量關閉選取的 %d 台主機", len(selectedIDs))

	case "delete":
		var updatedHosts []config.Host
		for _, h := range s.cfg.Hosts {
			if !idMap[h.ID] {
				updatedHosts = append(updatedHosts, h)
			}
		}
		s.cfg.Hosts = updatedHosts
		s.cfg.Save(s.configPath)
		logger.Log(logger.INFO, "", "已批量刪除選取的 %d 台主機", len(selectedIDs))
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// syncSingleHost 背景非同步更新單一主機 DNS
func (s *Server) syncSingleHost(ctx context.Context, h config.Host) {
	pConfig, ok := s.cfg.Providers[h.Provider]
	if !ok {
		logger.Log(logger.ERROR, h.ID, "找不到綁定的 DNS Provider: %s", h.Provider)
		return
	}

	p, err := provider.NewProvider(pConfig)
	if err != nil {
		logger.Log(logger.ERROR, h.ID, "初始化 Provider %s 失敗: %v", h.Provider, err)
		return
	}

	if h.IPv4.Enabled {
		ip4, err := ipfetcher.ResolveIP(h.IPv4, false, s.cfg.Defaults.IPv4APIs, h.ID)
		if err != nil {
			logger.Log(logger.ERROR, h.ID, "計算 IPv4 失敗: %v", err)
		} else {
			for _, domain := range h.Domains {
				if err := p.UpdateRecord(ctx, domain, provider.RecordA, ip4, h.Proxied); err != nil {
					logger.Log(logger.ERROR, h.ID, "更新 A 紀錄 %s 失敗: %v", domain, err)
				}
			}
		}
	}

	if h.IPv6.Enabled {
		ip6, err := ipfetcher.ResolveIP(h.IPv6, true, s.cfg.Defaults.IPv6APIs, h.ID)
		if err != nil {
			logger.Log(logger.ERROR, h.ID, "計算 IPv6 失敗: %v", err)
		} else {
			for _, domain := range h.Domains {
				if err := p.UpdateRecord(ctx, domain, provider.RecordAAAA, ip6, h.Proxied); err != nil {
					logger.Log(logger.ERROR, h.ID, "更新 AAAA 紀錄 %s 失敗: %v", domain, err)
				}
			}
		}
	}
}

// handleHostsBatchAdd 處理批量新增主機頁面渲染 (/hosts/batch-add)
func (s *Server) handleHostsBatchAdd(w http.ResponseWriter, r *http.Request) {
	data := PageData{
		ActiveTab:           "dashboard",
		Providers:           s.cfg.Providers,
		IsBatchEdit:         false,
		CommonIPv4Enabled:   true,
		CommonIPv4Mode:      "external_api",
		CommonIPv4Interface: "eth0",
		CommonIPv6Enabled:   true,
		CommonIPv6Mode:      "prefix_stitching",
		CommonIPv6Interface: "eth0",
	}
	s.renderTemplate(w, "batch_hosts.html", data)
}

// handleHostsBatchEdit 處理同 Provider 批量編輯頁面 (/hosts/batch-edit)
func (s *Server) handleHostsBatchEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	selectedIDs := r.Form["host_ids"]
	if len(selectedIDs) == 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	selectedMap := make(map[string]bool)
	for _, id := range selectedIDs {
		selectedMap[id] = true
	}

	var targetHosts []config.Host
	var firstProvider string
	for _, h := range s.cfg.Hosts {
		if selectedMap[h.ID] {
			if firstProvider == "" {
				firstProvider = h.Provider
			}
			if h.Provider == firstProvider {
				targetHosts = append(targetHosts, h)
			}
		}
	}

	if len(targetHosts) == 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// 統計出現頻率最高的 Mode
	v4ModeCount := make(map[string]int)
	v6ModeCount := make(map[string]int)
	for _, h := range targetHosts {
		if h.IPv4.Enabled {
			v4ModeCount[h.IPv4.Mode]++
		}
		if h.IPv6.Enabled {
			v6ModeCount[h.IPv6.Mode]++
		}
	}

	mostV4Mode := "external_api"
	maxV4 := 0
	for m, c := range v4ModeCount {
		if c > maxV4 {
			maxV4 = c
			mostV4Mode = m
		}
	}

	mostV6Mode := "prefix_stitching"
	maxV6 := 0
	for m, c := range v6ModeCount {
		if c > maxV6 {
			maxV6 = c
			mostV6Mode = m
		}
	}

	var batchRows []BatchRow
	excludedCount := 0
	for _, h := range targetHosts {
		if h.IPv4.Enabled && h.IPv4.Mode != mostV4Mode {
			excludedCount++
			continue
		}
		if h.IPv6.Enabled && h.IPv6.Mode != mostV6Mode {
			excludedCount++
			continue
		}

		v4Val := h.IPv4.Match
		if h.IPv4.Offset != "" {
			v4Val = h.IPv4.Offset
		}
		if h.IPv4.MAC != "" {
			v4Val = h.IPv4.MAC
		}
		if h.IPv4.URL != "" {
			v4Val = h.IPv4.URL
		}
		if h.IPv4.Command != "" {
			v4Val = h.IPv4.Command
		}

		v6Val := h.IPv6.Suffix
		if h.IPv6.MAC != "" {
			v6Val = h.IPv6.MAC
		}
		if h.IPv6.Match != "" {
			v6Val = h.IPv6.Match
		}
		if h.IPv6.URL != "" {
			v6Val = h.IPv6.URL
		}
		if h.IPv6.Command != "" {
			v6Val = h.IPv6.Command
		}

		batchRows = append(batchRows, BatchRow{
			ID:        h.ID,
			Name:      h.Name,
			Enabled:   h.Enabled,
			DomainStr: strings.Join(h.Domains, ","),
			IPv4Val:   v4Val,
			IPv6Val:   v6Val,
			Proxied:   h.Proxied,
		})
	}

	msg := ""
	if excludedCount > 0 {
		msg = fmt.Sprintf("已自動排除 %d 台模式 (Mode) 不符的主機，本次僅編輯相同模式的主機。", excludedCount)
	}

	data := PageData{
		ActiveTab:           "dashboard",
		Providers:           s.cfg.Providers,
		IsBatchEdit:         true,
		CommonProvider:      firstProvider,
		CommonIPv4Enabled:   true,
		CommonIPv4Mode:      mostV4Mode,
		CommonIPv4Interface: "eth0",
		CommonIPv6Enabled:   true,
		CommonIPv6Mode:      mostV6Mode,
		CommonIPv6Interface: "eth0",
		BatchRows:           batchRows,
		Message:             msg,
	}
	s.renderTemplate(w, "batch_hosts.html", data)
}

// handleHostsBatchSave 處理批量儲存主機列表動作 (/hosts/batch-save)
func (s *Server) handleHostsBatchSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	providerID := r.FormValue("common_provider")
	v4Enabled := r.FormValue("common_ipv4_enabled") == "true"
	v4Mode := r.FormValue("common_ipv4_mode")
	v4Iface := r.FormValue("common_ipv4_interface")

	v6Enabled := r.FormValue("common_ipv6_enabled") == "true"
	v6Mode := r.FormValue("common_ipv6_mode")
	v6Iface := r.FormValue("common_ipv6_interface")

	rowIndices := r.Form["row_index"]
	names := r.Form["row_name"]
	domainsList := r.Form["row_domains"]
	v4Vals := r.Form["row_ipv4_val"]
	v6Vals := r.Form["row_ipv6_val"]

	for i, idxStr := range rowIndices {
		if i >= len(names) || i >= len(domainsList) {
			continue
		}
		name := strings.TrimSpace(names[i])
		if name == "" {
			continue
		}

		domStr := strings.TrimSpace(domainsList[i])
		if domStr == "" {
			continue
		}
		rawDomains := strings.FieldsFunc(domStr, func(c rune) bool {
			return c == ',' || c == ';' || c == ' '
		})

		v4Val := ""
		if i < len(v4Vals) {
			v4Val = strings.TrimSpace(v4Vals[i])
		}
		v6Val := ""
		if i < len(v6Vals) {
			v6Val = strings.TrimSpace(v6Vals[i])
		}

		rowEnabled := r.FormValue("row_enabled_"+idxStr) == "true"
		rowProxied := r.FormValue("row_proxied_"+idxStr) == "true"

		v4Cfg := config.IPConfig{
			Enabled:   v4Enabled,
			Mode:      v4Mode,
			Interface: v4Iface,
		}
		switch v4Mode {
		case "external_api":
			v4Cfg.URL = v4Val
		case "interface":
			v4Cfg.Match = v4Val
		case "base_offset":
			v4Cfg.Offset = v4Val
		case "arp_mac":
			v4Cfg.MAC = v4Val
		case "command":
			v4Cfg.Command = v4Val
		}

		v6Cfg := config.IPConfig{
			Enabled:   v6Enabled,
			Mode:      v6Mode,
			Interface: v6Iface,
		}
		switch v6Mode {
		case "prefix_stitching":
			v6Cfg.Suffix = v6Val
		case "eui64_mac":
			v6Cfg.MAC = v6Val
		case "interface":
			v6Cfg.Match = v6Val
		case "external_api":
			v6Cfg.URL = v6Val
		case "command":
			v6Cfg.Command = v6Val
		}

		id := fmt.Sprintf("host-%d", len(s.cfg.Hosts)+1)
		host := config.Host{
			ID:       id,
			Name:     name,
			Enabled:  rowEnabled,
			Provider: providerID,
			Domains:  rawDomains,
			Proxied:  rowProxied,
			IPv4:     v4Cfg,
			IPv6:     v6Cfg,
		}

		found := false
		for idx, existing := range s.cfg.Hosts {
			if existing.Name == name || (len(existing.Domains) > 0 && len(rawDomains) > 0 && existing.Domains[0] == rawDomains[0]) {
				host.ID = existing.ID
				s.cfg.Hosts[idx] = host
				found = true
				break
			}
		}
		if !found {
			s.cfg.Hosts = append(s.cfg.Hosts, host)
		}
	}

	s.cfg.Save(s.configPath)
	logger.Log(logger.SUCCESS, "", "批量儲存主機完成！")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleHostsExportCSV 處理 CSV 匯出請求 (/hosts/export.csv)
func (s *Server) handleHostsExportCSV(w http.ResponseWriter, r *http.Request) {
	csvStr, err := s.cfg.ExportCSV()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"muddns_hosts.csv\"")
	w.Write([]byte(csvStr))
}

// handleHostsImportCSV 處理 CSV 匯入請求 (/hosts/import.csv)
func (s *Server) handleHostsImportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	content := r.FormValue("csv_content")
	count, err := s.cfg.ImportCSV(content)
	if err != nil {
		logger.Log(logger.ERROR, "", "CSV 匯入失敗: %v", err)
	} else {
		s.cfg.Save(s.configPath)
		logger.Log(logger.SUCCESS, "", "成功匯入 %d 台主機設定！", count)
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleProviders 處理 DNS 服務商清單頁面 (/providers)
func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	data := PageData{
		ActiveTab: "providers",
		Providers: s.cfg.Providers,
	}
	s.renderTemplate(w, "providers.html", data)
}

// handleProviderEdit 處理新增/編輯 DNS 服務商頁面 (/providers/edit)
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
	s.renderTemplate(w, "providers.html", data)
}

// handleProviderSave 處理 DNS 服務商儲存動作 (/providers/save)
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
	logger.Log(logger.INFO, "", "Provider %s (%s) 儲存成功", id, pType)

	http.Redirect(w, r, "/providers", http.StatusSeeOther)
}

// handleProviderDelete 處理 DNS 服務商刪除動作 (/providers/delete)
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
		logger.Log(logger.INFO, "", "Provider %s 刪除成功", id)
	}

	http.Redirect(w, r, "/providers", http.StatusSeeOther)
}

// handleConfigRaw 處理 RAW 文字檔編輯器頁面與儲存 (/config/raw)
func (s *Server) handleConfigRaw(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		r.ParseForm()
		raw := r.FormValue("raw_yaml")

		testCfg := &config.Config{}
		if err := yaml.Unmarshal([]byte(raw), testCfg); err != nil {
			data := PageData{
				ActiveTab: "config_raw",
				RawYAML:   raw,
				Error:     fmt.Sprintf("YAML 語法解析錯誤: %v", err),
			}
			s.renderTemplate(w, "config_raw.html", data)
			return
		}

		if err := os.WriteFile(s.configPath, []byte(raw), 0644); err != nil {
			data := PageData{
				ActiveTab: "config_raw",
				RawYAML:   raw,
				Error:     fmt.Sprintf("寫入設定檔失敗: %v", err),
			}
			s.renderTemplate(w, "config_raw.html", data)
			return
		}

		newCfg, err := config.LoadConfig(s.configPath)
		if err == nil {
			s.cfg = newCfg
		}

		logger.Log(logger.SUCCESS, "", "RAW config.yaml 已成功更新！")
		data := PageData{
			ActiveTab: "config_raw",
			RawYAML:   raw,
			Message:   "config.yaml 原始設定檔已成功儲存並重新載入！",
		}
		s.renderTemplate(w, "config_raw.html", data)
		return
	}

	rawBytes, _ := os.ReadFile(s.configPath)
	data := PageData{
		ActiveTab: "config_raw",
		RawYAML:   string(rawBytes),
	}
	s.renderTemplate(w, "config_raw.html", data)
}

// handleSettings 處理全域密碼變更與設定 (/settings)
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
			data.Error = "目前舊密碼不正確！"
			s.renderTemplate(w, "settings.html", data)
			return
		}

		if newPassword == "" {
			data.Error = "新密碼不能為空！"
			s.renderTemplate(w, "settings.html", data)
			return
		}

		if newPassword != confirmPassword {
			data.Error = "兩次輸入的新密碼不一致！"
			s.renderTemplate(w, "settings.html", data)
			return
		}

		newHash, err := config.HashPassword(newPassword)
		if err != nil {
			data.Error = fmt.Sprintf("密碼加密失敗: %v", err)
			s.renderTemplate(w, "settings.html", data)
			return
		}

		if newUsername != "" {
			s.cfg.Settings.WebAuth.Username = newUsername
		}
		s.cfg.Settings.WebAuth.PasswordHash = newHash

		if err := s.cfg.Save(s.configPath); err != nil {
			data.Error = fmt.Sprintf("儲存設定檔失敗: %v", err)
			s.renderTemplate(w, "settings.html", data)
			return
		}

		logger.Log(logger.SUCCESS, "", "管理員帳號密碼已成功更新！")
		data.WebAuth = s.cfg.Settings.WebAuth
		data.Message = "管理員帳號與密碼已成功更新！下次登入請使用新密碼。"
	}

	s.renderTemplate(w, "settings.html", data)
}

// handleLogs 處理系統日誌查詢 (/logs)
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	hostID := r.URL.Query().Get("host_id")
	level := r.URL.Query().Get("level")

	logs := logger.GlobalRingBuffer.GetLogs(hostID, level)

	data := PageData{
		ActiveTab: "logs",
		Logs:      logs,
	}
	s.renderTemplate(w, "logs.html", data)
}

// handlePreview 處理 HTMX 即時 IP 預覽 API (/api/preview)
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
		case "interface":
			ipCfg.Match = val
		case "command":
			ipCfg.Command = val
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
		case "command":
			ipCfg.Command = val
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

// handleLogout 處理管理員登出 (/logout)
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", `Basic realm="muddns"`)
	http.Error(w, "Logged out", http.StatusUnauthorized)
}
