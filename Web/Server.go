package Web

import (
	"AIxVuln/ProjectManager"
	"AIxVuln/misc"
	"AIxVuln/taskManager"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
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
	msgChan    chan ProjectManager.WebMsg
	accessHost string
}

func NewServer() *Server {
	return &Server{pms: make(map[string]*ProjectManager.ProjectManager), msgChan: make(chan ProjectManager.WebMsg, 10)}
}
func (s *Server) SaveProjectManagerToFile() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var pmsl []taskManager.ProjectInfo
	for _, pm := range s.pms {
		pmsl = append(pmsl, taskManager.ProjectInfo{
			ProjectName:   pm.GetProjectName(),
			SourceCodeDir: pm.GetSourceCodeDir(),
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
		projectConfig := ProjectManager.ProjectConfig{ProjectName: p.ProjectName, AnalyzeAgentNumber: 3, VerifierAgentNumber: 3, SourceCodeDir: p.SourceCodeDir, MsgChan: s.msgChan}
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
	// 启动时加载历史数据，保证重启后不丢失
	s.LoadProjectManagerFromFile()

	r := gin.Default()
	authorized := r.Group("/", gin.BasicAuth(gin.Accounts{
		"admin": "passwd",
	}))

	r.Use(cors.New(cors.Config{
		// 允许所有来源
		AllowOrigins: []string{"*"},

		// 或者指定特定的源
		// AllowOrigins: []string{"https://example.com", "http://localhost:3000"},

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
		AllowCredentials: true,

		// 预检请求缓存时间
		MaxAge: 12 * time.Hour,
	}))

	authorized.GET("/projects", s.getPms)
	authorized.GET("/projects/:name", s.getProject)
	authorized.POST("/projects/create", s.createProject)
	authorized.GET("/projects/:name/del", s.delProject)
	authorized.GET("/projects/:name/start", s.startProject)
	authorized.GET("/projects/:name/cancel", s.cancelProject)
	authorized.GET("/projects/:name/vulns", s.vulnList)
	authorized.GET("/projects/:name/containers", s.containerList)
	authorized.GET("/projects/:name/events", s.eventList)
	authorized.GET("/projects/:name/reports", s.reportList)
	authorized.GET("/projects/:name/envinfo", s.getEnvInfo)
	authorized.GET("/projects/:name/reports/download/:id", s.downloadReport)
	authorized.GET("/projects/:name/reports/downloadAll", s.downloadReportAll)
	authorized.GET("/", func(c *gin.Context) {
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

	httpServer := &http.Server{Addr: "0.0.0.0:" + port, Handler: r}
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
