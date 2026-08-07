package web

import (
	"io/fs"
	"net/http"
)

// RegisterRoutes 註冊所有 HTTP 路由與 URL 對應處理函式
// 採用 Go 1.22+ 原生標準庫語法："HTTP_METHOD /path"，可在一行內一目瞭然路由為 GET 還是 POST
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// Dashboard 主控板與單一主機管理
	mux.HandleFunc("GET /", s.requireAuth(s.handleDashboard))
	mux.HandleFunc("GET /hosts/edit", s.requireAuth(s.handleHostEdit))
	mux.HandleFunc("POST /hosts/save", s.requireAuth(s.handleHostSave))
	mux.HandleFunc("GET /hosts/sync-single", s.requireAuth(s.handleHostSyncSingle))
	mux.HandleFunc("POST /hosts/sync-single", s.requireAuth(s.handleHostSyncSingle))

	// 批量主機操作與大量新增/編輯
	mux.HandleFunc("POST /hosts/batch", s.requireAuth(s.handleHostBatch))
	mux.HandleFunc("GET /hosts/batch-add", s.requireAuth(s.handleHostsBatchAdd))
	mux.HandleFunc("POST /hosts/batch-edit", s.requireAuth(s.handleHostsBatchEdit))
	mux.HandleFunc("POST /hosts/batch-save", s.requireAuth(s.handleHostsBatchSave))

	// CSV 匯入與匯出
	mux.HandleFunc("GET /hosts/export.csv", s.requireAuth(s.handleHostsExportCSV))
	mux.HandleFunc("POST /hosts/import.csv", s.requireAuth(s.handleHostsImportCSV))

	// DNS 服務商管理 (Provider Manager)
	mux.HandleFunc("GET /providers", s.requireAuth(s.handleProviders))
	mux.HandleFunc("GET /providers/edit", s.requireAuth(s.handleProviderEdit))
	mux.HandleFunc("POST /providers/save", s.requireAuth(s.handleProviderSave))
	mux.HandleFunc("POST /providers/delete", s.requireAuth(s.handleProviderDelete))

	// RAW Config 原始 YAML 編輯器 (GET 載入頁面，POST 儲存檔)
	mux.HandleFunc("GET /config/raw", s.requireAuth(s.handleConfigRaw))
	mux.HandleFunc("POST /config/raw", s.requireAuth(s.handleConfigRaw))

	// 系統日誌與全局密碼設定
	mux.HandleFunc("GET /logs", s.requireAuth(s.handleLogs))
	mux.HandleFunc("GET /settings", s.requireAuth(s.handleSettings))
	mux.HandleFunc("POST /settings", s.requireAuth(s.handleSettings))

	// 靜態資源 (CSS 樣式表)
	subStatic, _ := fs.Sub(webFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(subStatic))))

	// 外部自動化與 OPNsense WAN 觸發同步 API (支援 GET / POST /api/sync)
	mux.HandleFunc("GET /api/sync", s.handleAPISync)
	mux.HandleFunc("POST /api/sync", s.handleAPISync)

	// HTMX 即時預覽 API 與登出
	mux.HandleFunc("POST /api/preview", s.requireAuth(s.handlePreview))
	mux.HandleFunc("GET /logout", s.handleLogout)
}

// requireAuth HTTP Basic Auth 中介層 (Middleware)，確保未驗證的請求被拒絕
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
