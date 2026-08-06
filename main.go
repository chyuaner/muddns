// Package main 為 muddns 專案的核心進入點 (Entry Point)。
// 本檔案保持極簡設計，僅設定版本號並呼叫 cli 模組進行命令派發。
package main

import (
	"muddns/cli"
)

// Version 程式版本號 (可在編譯時透過 -ldflags "-X main.Version=v1.2.0" 進行動態注入)
var Version = "v1.1.0"

func main() {
	// 將命令列參數交給 cli 模組進行解析與處理
	cli.Run(Version)
}
