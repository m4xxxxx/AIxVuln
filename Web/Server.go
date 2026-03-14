package Web

import (
	"AIxVuln/ProjectManager"
	"AIxVuln/misc"
	"AIxVuln/taskManager"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
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

	startQueueMu     sync.Mutex
	startQueue       []string
	startQueueSet    map[string]struct{}
	startRunningSet  map[string]struct{}
	startQueueNotify chan struct{}

	intelligentTaskMu    sync.RWMutex
	intelligentTasks     map[string]*intelligentAddTask
	intelligentTaskOrder []string
	stateStore           *projectStateStore
}

const (
	projectManagerPersistInterval = 20 * time.Second
	maxPersistProjectEvents       = 1200
	maxProjectManagerFileBytes    = 64 * 1024 * 1024 // 64MB safeguard
)

func NewServer() *Server {
	s := &Server{
		pms:              make(map[string]*ProjectManager.ProjectManager),
		msgChan:          make(chan string, 10),
		startQueue:       make([]string, 0),
		startQueueSet:    make(map[string]struct{}),
		startRunningSet:  make(map[string]struct{}),
		startQueueNotify: make(chan struct{}, 1),
		intelligentTasks: make(map[string]*intelligentAddTask),
	}
	if st, err := newProjectStateStore(misc.GetDataDir()); err != nil {
		misc.Debug("NewServer: init sqlite project state store failed: %v", err)
	} else {
		s.stateStore = st
	}
	s.initStartQueueDispatcher()
	return s
}
func (s *Server) SaveProjectManagerToFile() {
	s.mu.RLock()
	var pmsl []taskManager.ProjectInfo
	for _, pm := range s.pms {
		pmsl = append(pmsl, taskManager.ProjectInfo{
			ProjectName:   pm.GetProjectName(),
			SourceCodeDir: pm.GetSourceCodeDir(),
			TaskContent:   pm.GetTaskContent(),
			SandboxID:     pm.GetSandboxContainerID(),
			StartTime:     pm.GetStartTime(),
			EndTime:       pm.GetEndTime(),
			ContainerList: pm.GetContainerList(),
			VulnList:      pm.GetVulnList(),
			ExploitIdeas:  pm.GetExploitIdeaList(),
			ExploitChains: pm.GetExploitChainList(),
			TokenUsage:    pm.GetTokenUsageSnapshot(),
			EventList:     pm.GetEvent(maxPersistProjectEvents),
			EnvInfo:       pm.GetEnvInfo(),
			ProjectDir:    pm.GetProjectDir(),
			ReportList:    pm.GetReportList(),
		})
	}
	s.mu.RUnlock()
	if s.stateStore != nil {
		if err := s.stateStore.SaveAll(pmsl); err == nil {
			return
		} else {
			misc.Debug("SaveProjectManagerToFile: sqlite save failed, fallback to json: %v", err)
		}
	}
	s.saveProjectInfosToLegacyJSON(pmsl)
}
func (s *Server) LoadProjectManagerFromFile() {
	s.mu.Lock()
	defer s.mu.Unlock()

	var pmsl []taskManager.ProjectInfo
	if s.stateStore != nil {
		if loaded, err := s.stateStore.LoadAll(); err != nil {
			misc.Debug("LoadProjectManagerFromFile: sqlite load failed: %v", err)
		} else if len(loaded) > 0 {
			pmsl = loaded
			misc.Debug("LoadProjectManagerFromFile: restored %d projects from sqlite", len(pmsl))
		}
	}

	legacyPath := filepath.Join(misc.GetDataDir(), "projectManager.json")
	if len(pmsl) == 0 {
		legacyLoaded, err := s.loadProjectInfosFromLegacyJSON(legacyPath)
		if err == nil && len(legacyLoaded) > 0 {
			pmsl = legacyLoaded
			misc.Debug("LoadProjectManagerFromFile: restored %d projects from legacy json", len(pmsl))
			if s.stateStore != nil {
				if err := s.stateStore.SaveAll(pmsl); err != nil {
					misc.Debug("LoadProjectManagerFromFile: migrate legacy json to sqlite failed: %v", err)
				} else {
					backup := fmt.Sprintf("%s.migrated.%s.bak", legacyPath, time.Now().Format("20060102_150405"))
					_ = os.Rename(legacyPath, backup)
					misc.Debug("LoadProjectManagerFromFile: migrated legacy json to sqlite, backup=%s", backup)
				}
			}
		}
	}

	if len(pmsl) == 0 {
		return
	}

	for _, p := range pmsl {
		if len(p.EventList) > maxPersistProjectEvents {
			p.EventList = append([]string(nil), p.EventList[len(p.EventList)-maxPersistProjectEvents:]...)
		}
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
		pm.SetSandboxContainerID(p.SandboxID)
		pm.RestoreExploitState(p.ExploitIdeas, p.ExploitChains)
		pm.RestoreTokenUsageSnapshot(p.TokenUsage)
		s.pms[p.ProjectName] = pm
	}
}

func (s *Server) saveProjectInfosToLegacyJSON(pmsl []taskManager.ProjectInfo) {
	ps, err := json.Marshal(pmsl)
	if err != nil {
		return
	}
	target := filepath.Join(misc.GetDataDir(), "projectManager.json")
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, ps, 0644); err != nil {
		return
	}
	_ = os.Rename(tmp, target)
}

func (s *Server) loadProjectInfosFromLegacyJSON(pmPath string) ([]taskManager.ProjectInfo, error) {
	if st, err := os.Stat(pmPath); err == nil && st.Size() > maxProjectManagerFileBytes {
		backup := fmt.Sprintf("%s.too_large.%s.bak", pmPath, time.Now().Format("20060102_150405"))
		_ = os.Rename(pmPath, backup)
		return nil, fmt.Errorf("legacy snapshot too large (%d bytes), moved to %s", st.Size(), backup)
	}
	ps, err := os.ReadFile(pmPath)
	if err != nil {
		return nil, err
	}
	var pmsl []taskManager.ProjectInfo
	if err := json.Unmarshal(ps, &pmsl); err != nil {
		return nil, err
	}
	return pmsl, nil
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
	authorized.POST("/projects/intelligent_add_open_source_audit/start", s.startIntelligentAddOpenSourceAuditTask)
	authorized.GET("/projects/intelligent_add_open_source_audit/tasks/:id", s.getIntelligentAddOpenSourceAuditTask)
	authorized.POST("/projects/intelligent_add_open_source_audit", s.intelligentAddOpenSourceAudit)
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
		ticker := time.NewTicker(projectManagerPersistInterval)
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
	fmt.Println("AIxVuln Web Server started")
	fmt.Println("  ➜ Local:   http://127.0.0.1:" + port)
	fmt.Println("  ➜ Network: http://0.0.0.0:" + port)
	go func() {
		_ = httpServer.ListenAndServe()
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// 退出前持久化一次并优雅关闭
	s.SaveProjectManagerToFile()
	if s.stateStore != nil {
		if err := s.stateStore.Close(); err != nil {
			misc.Debug("startWebServer: close sqlite project state store failed: %v", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	fmt.Println("web server shutdown")
}
