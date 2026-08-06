package web

import (
	"embed"
)

// 使用 Go 1.16+ 特性，在編譯時自動將 templates 與 static 目錄下的所有檔案打包至二進位檔
//go:embed templates static
var webFS embed.FS

// GetWebFS 回傳包含 static 與 templates 的靜態資產檔案系統實體
func GetWebFS() embed.FS {
	return webFS
}
