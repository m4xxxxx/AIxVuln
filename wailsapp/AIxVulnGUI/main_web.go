//go:build !(desktop || bindings)

package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"

	"AIxVuln/Web"
	"AIxVuln/misc"
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

	port := flag.String("port", "9999", "http listen port")
	flag.Parse()

	uiFS, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}
	srv := Web.NewServer()
	srv.StartWebServerWithUIFS(*port, uiFS)
}
