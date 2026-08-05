package logger

import (
	"fmt"

	"io"

	"log"

	"os"

	"strings"

	"sync"

	"time"
)

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	SUCCESS
)

func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case SUCCESS:
		return "SUCCESS"
	default:
		return "INFO"
	}
}

type LogEntry struct {
	Timestamp string   `json:"timestamp"`
	Level     string   `json:"level"`
	HostID    string   `json:"host_id,omitempty"`
	Message   string   `json:"message"`
}

type RingBuffer struct {
	mu       sync.RWMutex
	entries  []LogEntry
	capacity int
}

func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		entries:  make([]LogEntry, 0, capacity),
		capacity: capacity,
	}
}

func (r *RingBuffer) Add(entry LogEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.entries) >= r.capacity {
		r.entries = r.entries[1:]
	}
	r.entries = append(r.entries, entry)
}

func (r *RingBuffer) GetLogs(hostIDFilter string, levelFilter string) []LogEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := make([]LogEntry, 0, len(r.entries))
	for i := len(r.entries) - 1; i >= 0; i-- {
		entry := r.entries[i]
		if hostIDFilter != "" && entry.HostID != hostIDFilter {
			continue
		}
		if levelFilter != "" && !strings.EqualFold(entry.Level, levelFilter) {
			continue
		}
		res = append(res, entry)
	}
	return res
}

var (
	GlobalRingBuffer = NewRingBuffer(500)
	VerboseDebug     = false
	stdLogger        = log.New(os.Stdout, "", log.LstdFlags)
)

func SetLogFile(filePath string) error {
	if filePath == "" {
		return nil
	}
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	mw := io.MultiWriter(os.Stdout, f)
	stdLogger = log.New(mw, "", log.LstdFlags)
	return nil
}

func Log(level LogLevel, hostID string, format string, args ...interface{}) {
	if level == DEBUG && !VerboseDebug {
		return
	}

	msg := fmt.Sprintf(format, args...)
	timeStr := time.Now().Format("2006-01-02 15:04:05")

	entry := LogEntry{
		Timestamp: timeStr,
		Level:     level.String(),
		HostID:    hostID,
		Message:   msg,
	}
	GlobalRingBuffer.Add(entry)

	prefix := fmt.Sprintf("[%s]", level.String())
	if hostID != "" {
		prefix = fmt.Sprintf("[%s] [%s]", level.String(), hostID)
	}
	stdLogger.Printf("%s %s", prefix, msg)
}
