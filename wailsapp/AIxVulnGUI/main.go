//go:build desktop || bindings

package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"AIxVuln/Web"
	"AIxVuln/misc"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed all:dockerfile
var dockerfileFS embed.FS

func init() {
	var err error
	err = misc.CreateDirIfNotExists("data/temp/")
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	// Extract embedded dockerfiles for docker build support.
	if sub, err := fs.Sub(dockerfileFS, "dockerfile"); err == nil {
		misc.SetDockerfileFS(sub)
	}
	defer misc.CleanupDockerfiles()
	mode := flag.String("mode", "gui", "mode: gui (wails) or web (gin serves dist)")
	port := flag.String("port", "9999", "http port for web mode")
	flag.Parse()

	if *mode == "web" {
		uiFS, err := fs.Sub(assets, "frontend/dist")
		if err != nil {
			log.Fatal(err)
		}
		srv := Web.NewServer()
		srv.StartWebServerWithUIFS(*port, uiFS)
		return
	}

	server := Web.NewServer()
	ginHandler := server.Handler(nil)

	// Create an instance of the app structure
	app := NewApp()
	app.SetGinHandler(ginHandler)

	// Create application with options
	var err error
	err = wails.Run(&options.App{
		Title:     "AIxVuln",
		Width:     1024,
		Height:    768,
		MinWidth:  900,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			// Defining Assets enables the Wails Frontend DevServer integration in `wails dev`.
			Assets: assets,
			// Route API + WebSocket to Gin handler, and let the default handler serve the UI.
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					p := r.URL.Path
					// Route all API / WebSocket paths to Gin; everything else to the frontend asset server.
					apiPrefixes := []string{"/projects", "/ws", "/healthz", "/login", "/init_status", "/init", "/config",
						"/digital_humans", "/report_templates", "/docker_build", "/docker_pull",
						"/models", "/avatar"}
					for _, prefix := range apiPrefixes {
						if p == prefix || strings.HasPrefix(p, prefix+"/") || strings.HasPrefix(p, prefix+"?") {
							ginHandler.ServeHTTP(w, r)
							return
						}
					}
					next.ServeHTTP(w, r)
				})
			},
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
