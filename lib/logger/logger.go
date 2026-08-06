// Package logger 提供了專案的全域日誌紀錄功能，包含主機日誌過濾與記憶體環狀緩衝區 (Ring Buffer)。
package logger

import (
	"fmt"
	"sync"
	"time"
)

// LogLevel 定義日誌層級 (INFO, SUCCESS, WARN, ERROR)
type LogLevel string

const (
	INFO    LogLevel = "INFO"
	SUCCESS LogLevel = "SUCCESS"
	WARN    LogLevel = "WARN"
	ERROR   LogLevel = "ERROR"
)

// VerboseDebug 記錄是否開啟詳細除錯模式
var VerboseDebug bool

// LogEntry 代表單一條日誌紀錄結構
type LogEntry struct {
	Timestamp string   `json:"timestamp"` // 記錄時間 (例: 2026/08/06 14:30:00)
	Level     LogLevel `json:"level"`     // 日誌層級
	HostID    string   `json:"host_id"`   // 關聯的主機 ID (非主機日誌則為空)
	Message   string   `json:"message"`   // 日誌訊息內文
}

// LogBuffer 代表記憶體中固定容量的環狀日誌緩衝區
type LogBuffer struct {
	mu       sync.Mutex // 確保多執行緒讀寫安全
	entries  []LogEntry // 存放日誌陣列
	capacity int        // 最大保留條數 (預設 500 條)
}

// GlobalRingBuffer 全域共用的日誌緩衝區實例
var GlobalRingBuffer = NewLogBuffer(500)

// NewLogBuffer 初始化建立指定容量的日誌緩衝區
func NewLogBuffer(capacity int) *LogBuffer {
	return &LogBuffer{
		entries:  make([]LogEntry, 0, capacity),
		capacity: capacity,
	}
}

// Add 向緩衝區加入一條新日誌 (若超過最大容量，會自動剔除最舊的一條日誌)
func (b *LogBuffer) Add(entry LogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.entries) >= b.capacity {
		// 移除第 0 條 (最舊)，保留後續條目
		b.entries = b.entries[1:]
	}
	b.entries = append(b.entries, entry)
}

// GetLogs 取得篩選後的日誌清單 (可指定 hostID 或 level 進行過濾)
func (b *LogBuffer) GetLogs(hostID string, level string) []LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()

	var result []LogEntry
	// 倒序遍歷，讓最新日誌排在最前
	for i := len(b.entries) - 1; i >= 0; i-- {
		e := b.entries[i]
		if hostID != "" && e.HostID != hostID {
			continue
		}
		if level != "" && string(e.Level) != level {
			continue
		}
		result = append(result, e)
	}
	return result
}

// Log 格式化輸出並記錄日誌至標準輸出與全域緩衝區
func Log(level LogLevel, hostID string, format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	nowStr := time.Now().Format("2006/01/02 15:04:05")

	entry := LogEntry{
		Timestamp: nowStr,
		Level:     level,
		HostID:    hostID,
		Message:   msg,
	}

	GlobalRingBuffer.Add(entry)

	// 格式化 Terminal 終端機輸出
	if hostID != "" {
		fmt.Printf("%s [%s] [%s] %s\n", nowStr, level, hostID, msg)
	} else {
		fmt.Printf("%s [%s] %s\n", nowStr, level, msg)
	}
}
