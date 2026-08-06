package web

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"

	"muddns/config"
	"muddns/lib/logger"
)

// WebServer 包覆核心 Server 與底層 http.Server 生命週期
type WebServer struct {
	*Server
	httpServer *http.Server
}

// NewServer 初始化建立 WebServer 實體並註冊路由
func NewServer(cfg *config.Config, configPath string, tmplFS fs.FS) (*WebServer, error) {
	tmpl, err := template.ParseFS(tmplFS, "templates/components/*.html", "templates/pages/*.html")
	if err != nil {
		return nil, fmt.Errorf("解析 HTML 範本失敗: %w", err)
	}

	baseServer := &Server{
		cfg:        cfg,
		configPath: configPath,
		tmpl:       tmpl,
	}

	mux := http.NewServeMux()
	baseServer.RegisterRoutes(mux)

	httpSrv := &http.Server{
		Addr:    cfg.Settings.Listen,
		Handler: mux,
	}

	return &WebServer{
		Server:     baseServer,
		httpServer: httpSrv,
	}, nil
}

// Start 啟動 Web 伺服器監聽服務
func (s *WebServer) Start() error {
	logger.Log(logger.INFO, "", "muddns web 伺服器已啟動，監聽於 %s", s.cfg.Settings.Listen)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown 安全關閉 Web 伺服器
func (s *WebServer) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
