package main

import (
	"AIxVuln/Web"
	"AIxVuln/misc"
	"embed"
	"io/fs"
)

//go:embed all:dockerfile
var dockerfileFS embed.FS

func init() {
	sub, err := fs.Sub(dockerfileFS, "dockerfile")
	if err != nil {
		misc.PanicWithStack("embed dockerfile 失败: %v", err)
	}
	misc.SetDockerfileFS(sub)
	err = misc.CreateDirIfNotExists("data/temp/")
	if err != nil {
		misc.PanicWithStack("创建 data/temp 失败: %v", err)
	}
}

func main() {
	defer misc.CleanupDockerfiles()
	server := Web.NewServer()
	server.StartWebServer("9999")
}
