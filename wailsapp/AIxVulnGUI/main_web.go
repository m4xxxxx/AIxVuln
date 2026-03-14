//go:build !(desktop || bindings)

package main

import (
	"AIxVuln/Web"
	"AIxVuln/misc"
	"embed"
	"flag"
	"io/fs"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed all:dockerfile
var dockerfileFS embed.FS

func init() {
	var err error
	err = misc.CreateDirIfNotExists("data/temp/")
	if err != nil {
		misc.PanicWithStack("创建 data/temp 失败: %v", err)
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
		misc.PanicWithStack("加载前端静态资源失败: %v", err)
	}
	srv := Web.NewServer()
	srv.StartWebServerWithUIFS(*port, uiFS)
}
