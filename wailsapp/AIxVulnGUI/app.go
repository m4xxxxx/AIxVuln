//go:build desktop || bindings

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
)

// App struct
type App struct {
	ctx        context.Context
	apiBaseURL string
	ginHandler http.Handler
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// SetGinHandler sets the shared Gin handler so the standalone HTTP listener
// (needed for WebSocket) uses the same server instance as the Wails middleware.
func (a *App) SetGinHandler(h http.Handler) {
	a.ginHandler = h
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods.
// It also starts a standalone HTTP listener on a free port so that WebSocket
// connections (which Wails AssetServer cannot proxy) work correctly.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if a.ginHandler == nil {
		return
	}
	port := preferPortOrFree(9999)
	a.apiBaseURL = "http://127.0.0.1:" + strconv.Itoa(port)
	go func() {
		_ = http.ListenAndServe("127.0.0.1:"+strconv.Itoa(port), a.ginHandler)
	}()
}

// Greet returns a greeting for the given name
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}

func (a *App) GetAPIBaseURL() string {
	return a.apiBaseURL
}

// GetBasicAuthUser and GetBasicAuthPassword return empty strings.
// Users are now managed in SQLite; the frontend uses the init wizard + token login flow.
func (a *App) GetBasicAuthUser() string {
	return ""
}

func (a *App) GetBasicAuthPassword() string {
	return ""
}

func preferPortOrFree(prefer int) int {
	if canListen(prefer) {
		return prefer
	}
	return findFreePort()
}

func canListen(port int) bool {
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

func findFreePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 9999
	}
	defer l.Close()
	addr := l.Addr().(*net.TCPAddr)
	return addr.Port
}
