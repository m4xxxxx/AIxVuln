package Web

import (
	"AIxVuln/ProjectManager"
	"AIxVuln/misc"
	"AIxVuln/taskManager"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type Server struct {
	mu         sync.RWMutex
	pms        map[string]*ProjectManager.ProjectManager
	msgChan    chan string
	accessHost string
}

func NewServer() *Server {
	return &Server{pms: make(map[string]*ProjectManager.ProjectManager), msgChan: make(chan string, 10)}
}
func (s *Server) SaveProjectManagerToFile() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var pmsl []taskManager.ProjectInfo
	for _, pm := range s.pms {
		pmsl = append(pmsl, taskManager.ProjectInfo{
			ProjectName:   pm.GetProjectName(),
			SourceCodeDir: pm.GetSourceCodeDir(),
			TaskContent:   pm.GetTaskContent(),
			StartTime:     pm.GetStartTime(),
			EndTime:       pm.GetEndTime(),
			ContainerList: pm.GetContainerList(),
			VulnList:      pm.GetVulnList(),
			EventList:     pm.GetEvent(0),
			EnvInfo:       pm.GetEnvInfo(),
			ProjectDir:    pm.GetProjectDir(),
			ReportList:    pm.GetReportList(),
		})
	}
	ps, err := json.Marshal(pmsl)
	if err != nil {
		return
	}
	_ = os.WriteFile(misc.GetDataDir()+"/projectManager.json", ps, 0644)
}
func (s *Server) LoadProjectManagerFromFile() {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps, err := ioutil.ReadFile(misc.GetDataDir() + "/projectManager.json")
	var pmsl []taskManager.ProjectInfo
	if err != nil {
		return
	}
	err = json.Unmarshal(ps, &pmsl)
	if err != nil {
		return
	}
	for _, p := range pmsl {
		projectConfig := ProjectManager.ProjectConfig{ProjectName: p.ProjectName, SourceCodeDir: p.SourceCodeDir, MsgChan: s.msgChan, TaskContent: p.TaskContent}
		pm, err := ProjectManager.NewProjectManager(projectConfig)
		if err != nil {
			continue
		}
		pm.SetEnvInfo(p.EnvInfo)
		pm.SetStatus("运行结束")
		pm.SetVulns(p.VulnList)
		pm.SetProjectDir(p.ProjectDir)
		pm.SetEvent(p.EventList)
		pm.SetContainer(p.ContainerList)
		pm.SetStartTime(p.StartTime)
		pm.SetEndTime(p.EndTime)
		pm.SetReport(p.ReportList)
		s.pms[p.ProjectName] = pm
	}
}

func (s *Server) StartWebServer(port string) {
	s.startWebServer(port, nil)
}

// StartWebServerWithUIFS starts the API server and also serves a built frontend (dist) at '/'.
// The UI is served without auth; API endpoints remain BasicAuth-protected.
// uiFS should contain the files under dist root (e.g. index.html, assets/*).
func (s *Server) StartWebServerWithUIFS(port string, uiFS fs.FS) {
	s.startWebServer(port, uiFS)
}

// Handler builds a gin handler for API routes.
// If uiFS is provided, it also serves the built frontend at '/' with SPA fallback.
func (s *Server) Handler(uiFS fs.FS) http.Handler {
	// 启动时加载历史数据，保证重启后不丢失
	s.LoadProjectManagerFromFile()
	gin.SetMode(gin.ReleaseMode)

	r := gin.Default()
	// Avoid automatic 301 redirects (e.g. path normalization) which can cause redirect loops
	// under some proxies / clients.
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false

	if uiFS != nil {
		ui := http.FS(uiFS)
		serveIndex := func(c *gin.Context) {
			b, err := fs.ReadFile(uiFS, "index.html")
			if err != nil {
				c.Status(500)
				return
			}
			c.Data(200, "text/html; charset=utf-8", b)
		}
		// Serve index at root.
		r.GET("/", func(c *gin.Context) {
			serveIndex(c)
		})
		// SPA fallback: any unknown route should return index.html.
		// We intentionally do NOT register a catch-all route like '/*filepath' (StaticFS on '/')
		// because it conflicts with API routes like '/projects'.
		r.NoRoute(func(c *gin.Context) {
			p := c.Request.URL.Path
			trimmed := strings.TrimPrefix(p, "/")
			// If request looks like an asset path, try to serve it from dist.
			if strings.Contains(path.Base(p), ".") {
				f, err := uiFS.Open(trimmed)
				if err != nil {
					c.Status(404)
					return
				}
				_ = f.Close()
				c.FileFromFS(trimmed, ui)
				return
			}
			// Otherwise treat as SPA route.
			serveIndex(c)
		})
	}

	// CORS must be applied before auth middleware; also do NOT use wildcard origin with credentials.
	r.Use(cors.New(cors.Config{
		AllowOriginFunc: func(origin string) bool { return true },

		// 允许的方法
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},

		// 允许的头部
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Content-Length",
			"Accept-Encoding",
			"X-CSRF-Token",
			"Authorization",
			"Accept",
			"Cache-Control",
			"X-Requested-With",
		},

		// 允许暴露的头部
		ExposeHeaders: []string{"Content-Length"},

		// 允许携带凭证
		AllowCredentials: false,

		// 预检请求缓存时间
		MaxAge: 12 * time.Hour,
	}))

	// Preflight should not require auth.
	r.OPTIONS("/*path", func(c *gin.Context) {
		c.Status(204)
	})

	// Login endpoint (no auth required).
	r.POST("/login", loginHandler)

	// Init endpoints (no auth required, guarded by HasAnyUser check).
	r.GET("/init_status", initStatusHandler)
	r.POST("/init", initSetupHandler)
	r.POST("/docker_build/:name", dockerBuildHandler)
	r.POST("/docker_pull/:name", dockerPullHandler)

	authorized := r.Group("/", tokenAuthMiddleware())

	authorized.GET("/projects", s.getPms)
	authorized.GET("/projects/:name", s.getProject)
	authorized.POST("/projects/create", s.createProject)
	authorized.GET("/projects/:name/del", s.delProject)
	authorized.GET("/projects/:name/start", s.startProject)
	authorized.GET("/projects/:name/cancel", s.cancelProject)
	authorized.GET("/projects/:name/agents", s.agentList)
	authorized.GET("/projects/:name/exploitIdeas", s.exploitIdeaList)
	authorized.GET("/projects/:name/exploitChains", s.exploitChainList)
	authorized.GET("/projects/:name/containers", s.containerList)
	authorized.GET("/projects/:name/events", s.eventList)
	authorized.GET("/projects/:name/reports", s.reportList)
	authorized.GET("/projects/:name/envinfo", s.getEnvInfo)
	authorized.GET("/projects/:name/reports/download/:id", s.downloadReport)
	authorized.GET("/projects/:name/reports/downloadAll", s.downloadReportAll)
	authorized.POST("/projects/:name/chat", s.teamChat)
	authorized.GET("/projects/:name/chat/messages", s.getChatMessages)
	authorized.GET("/projects/:name/token_usage", s.getTokenUsage)
	authorized.GET("/projects/:name/context_breakdown", s.getContextBreakdown)
	authorized.GET("/config", s.getConfig)
	authorized.PUT("/config", s.setConfig)
	authorized.GET("/models", s.listModels)
	authorized.GET("/digital_humans", s.getDigitalHumans)
	authorized.POST("/digital_humans", s.saveDigitalHuman)
	authorized.DELETE("/digital_humans/:id", s.deleteDigitalHuman)
	authorized.GET("/report_templates", s.getReportTemplates)
	authorized.PUT("/report_templates", s.setReportTemplate)
	authorized.POST("/avatar/upload", s.uploadAvatar)
	r.GET("/avatar/:name", serveAvatar)
	authorized.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"msg": "ok"})
	})

	manager := &ClientManager{
		clients: make(map[string]map[*websocket.Conn]bool),
	}
	r.GET("/ws", func(c *gin.Context) {
		handleWebSocket(c, manager)
	})
	go startBroadcasting(manager, s.msgChan)

	// 运行时定时持久化，避免异常退出导致状态丢失
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			s.SaveProjectManagerToFile()
		}
	}()

	return r
}

func (s *Server) startWebServer(port string, uiFS fs.FS) {
	h := s.Handler(uiFS)
	httpServer := &http.Server{Addr: "0.0.0.0:" + port, Handler: h}
	go func() {
		_ = httpServer.ListenAndServe()
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// 退出前持久化一次并优雅关闭
	s.SaveProjectManagerToFile()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	fmt.Println("web server shutdown")
}
