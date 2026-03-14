package misc

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	debugEnabled  bool
	debugInitOnce sync.Once
)

// initDebugFlag reads the DEBUG config from DB once.
func initDebugFlag() {
	debugInitOnce.Do(func() {
		val := strings.TrimSpace(dbGet("misc", "DEBUG"))
		debugEnabled = val == "true" || val == "1"
	})
}

// ReloadDebugFlag forces re-reading the DEBUG config (call after settings change).
func ReloadDebugFlag() {
	val := strings.TrimSpace(dbGet("misc", "DEBUG"))
	debugEnabled = val == "true" || val == "1"
}

func Info(mod string, msg string, eventHandler func(string, string, int)) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	m := fmt.Sprintf("[%s][*][%s]: %s", timestamp, mod, msg)
	if eventHandler != nil {
		eventHandler(mod, msg, 1)
		return
	}
	fmt.Println(m)
}

func Success(mod string, msg string, eventHandler func(string, string, int)) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	m := fmt.Sprintf("[%s][+][%s]: %s", timestamp, mod, msg)
	if eventHandler != nil {
		eventHandler(mod, msg, 1)
		return
	}
	fmt.Println(m)
}

func Warn(mod string, msg string, eventHandler func(string, string, int)) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	m := fmt.Sprintf("[%s][!][%s]: %s", timestamp, mod, msg)
	if eventHandler != nil {
		eventHandler(mod, msg, 2)
		return
	}
	fmt.Println(m)
}

func Error(mod string, msg string, eventHandler func(string, string, int)) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	m := fmt.Sprintf("[%s][-][%s]: %s", timestamp, mod, msg)
	if eventHandler != nil {
		eventHandler(mod, msg, 3)
		return
	}
	fmt.Println(m)
}

func Debug(format string, v ...any) {
	initDebugFlag()
	if !debugEnabled {
		return
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	m := fmt.Sprintf("[%s][DEBUG]: %s\n", timestamp, format)
	fmt.Println(fmt.Sprintf(m, v...))
}
