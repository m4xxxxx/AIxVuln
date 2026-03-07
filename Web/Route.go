package Web

import (
	"AIxVuln/DecisionBrain"
	"AIxVuln/ProjectManager"
	"AIxVuln/misc"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (s *Server) getPms(c *gin.Context) {
	var res []string
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.pms {
		res = append(res, v.GetProjectName())
	}
	c.JSON(200, Success(res))
}

func (s *Server) containerList(c *gin.Context) {
	s.mu.RLock()
	pm, exists := s.pms[c.Param("name")]
	s.mu.RUnlock()
	if !exists {
		c.JSON(404, Fail("project not found"))
		return
	}
	var cl []Container
	for _, co := range pm.GetContainerList() {
		if len(co.WebPort) > 0 {
			cl = append(cl, Container{ContainerId: co.ContainerId, ContainerIP: co.ContainerIP, Image: co.Image, WebPort: co.WebPort})
		} else {
			cl = append(cl, Container{ContainerId: co.ContainerId, ContainerIP: co.ContainerIP, Image: co.Image, WebPort: nil})
		}

	}
	c.JSON(200, Success(cl))
}

func (s *Server) agentList(c *gin.Context) {
	s.mu.RLock()
	pm, exists := s.pms[c.Param("name")]
	s.mu.RUnlock()
	if !exists {
		c.JSON(404, Fail("project not found"))
		return
	}
	c.JSON(200, Success(pm.GetDigitalHumanRoster()))
}

func (s *Server) exploitIdeaList(c *gin.Context) {
	s.mu.RLock()
	pm, exists := s.pms[c.Param("name")]
	s.mu.RUnlock()
	if !exists {
		c.JSON(404, Fail("project not found"))
		return
	}
	c.JSON(200, Success(pm.GetExploitIdeaList()))
}

func (s *Server) exploitChainList(c *gin.Context) {
	s.mu.RLock()
	pm, exists := s.pms[c.Param("name")]
	s.mu.RUnlock()
	if !exists {
		c.JSON(404, Fail("project not found"))
		return
	}
	c.JSON(200, Success(pm.GetExploitChainList()))
}

func (s *Server) vulnList(c *gin.Context) {
	s.mu.RLock()
	pm, exists := s.pms[c.Param("name")]
	s.mu.RUnlock()
	if !exists {
		c.JSON(404, Fail("project not found"))
		return
	}
	res := make(map[string]any)
	res["exploitIdeaList"] = pm.GetExploitIdeaList()
	res["exploitChainList"] = pm.GetExploitChainList()
	c.JSON(200, Success(res))
}

func (s *Server) eventList(c *gin.Context) {
	s.mu.RLock()
	pm, exists := s.pms[c.Param("name")]
	s.mu.RUnlock()
	if !exists {
		c.JSON(404, Fail("project not found"))
		return
	}
	count := c.DefaultQuery("count", "50")
	countInt, err := strconv.Atoi(count)
	if err != nil {
		c.JSON(400, Fail("count not int"))
		return
	}
	c.JSON(200, Success(pm.GetEvent(countInt)))
}

func (s *Server) reportList(c *gin.Context) {
	s.mu.RLock()
	pm, exists := s.pms[c.Param("name")]
	s.mu.RUnlock()
	if !exists {
		c.JSON(404, Fail("project not found"))
		return
	}
	report := make(map[string]string)
	for k, v := range pm.GetReportList() {
		report[k] = filepath.Base(v)
	}
	c.JSON(200, Success(report))
}

func (s *Server) downloadReport(c *gin.Context) {
	s.mu.RLock()
	pm, exists := s.pms[c.Param("name")]
	s.mu.RUnlock()
	if !exists {
		c.JSON(404, Fail("project not found"))
		return
	}
	reportList := pm.GetReportList()
	id := c.Param("id")
	p, e := reportList[id]
	if !e {
		c.JSON(404, Fail("report not found"))
		return
	}
	_, err := os.Stat(p)
	if err != nil {
		c.JSON(404, Fail("file not found"))
	}
	c.Header("Content-Disposition", "attachment; filename="+filepath.Base(p))
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Transfer-Encoding", "binary")
	c.File(p)
}

func (s *Server) downloadReportAll(c *gin.Context) {
	s.mu.RLock()
	pm, exists := s.pms[c.Param("name")]
	s.mu.RUnlock()
	if !exists {
		c.JSON(404, Fail("project not found"))
		return
	}
	reportDir := filepath.Join(pm.GetProjectDir(), "vulns")
	tempDir, err := filepath.Abs(filepath.Join(misc.GetDataDir(), "temp"))
	if err != nil {
		c.JSON(400, Fail("get temp zipfile error"))
		return
	}
	tempZipfile := filepath.Join(tempDir, uuid.New().String()+".zip")
	err = misc.ZipDirectory(reportDir, tempZipfile)
	if err != nil {
		c.JSON(400, Fail("zip file error"))
		return
	}
	defer os.Remove(tempZipfile)
	c.Header("Content-Disposition", "attachment; filename="+pm.GetProjectName()+"-report.zip")
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Transfer-Encoding", "binary")
	c.File(tempZipfile)
}

func (s *Server) teamChat(c *gin.Context) {
	s.mu.RLock()
	pm, exists := s.pms[c.Param("name")]
	s.mu.RUnlock()
	if !exists {
		c.JSON(404, Fail("project not found"))
		return
	}
	var body struct {
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Message == "" {
		c.JSON(400, Fail("message is required"))
		return
	}
	// Persist user message before sending.
	pm.AppendChatMessage(DecisionBrain.ChatMessage{Role: "user", Text: body.Message, Ts: time.Now().Format("15:04:05")})
	result := pm.TeamChat(body.Message)
	c.JSON(200, Success(result))
}

func (s *Server) getChatMessages(c *gin.Context) {
	s.mu.RLock()
	pm, exists := s.pms[c.Param("name")]
	s.mu.RUnlock()
	if !exists {
		c.JSON(404, Fail("project not found"))
		return
	}
	msgs := pm.GetChatMessages()
	if msgs == nil {
		msgs = make([]DecisionBrain.ChatMessage, 0)
	}
	c.JSON(200, Success(msgs))
}

func (s *Server) getTokenUsage(c *gin.Context) {
	s.mu.RLock()
	pm, exists := s.pms[c.Param("name")]
	s.mu.RUnlock()
	if !exists {
		c.JSON(404, Fail("project not found"))
		return
	}
	c.JSON(200, Success(pm.GetTokenUsage()))
}

func (s *Server) getContextBreakdown(c *gin.Context) {
	s.mu.RLock()
	pm, exists := s.pms[c.Param("name")]
	s.mu.RUnlock()
	if !exists {
		c.JSON(404, Fail("project not found"))
		return
	}
	c.JSON(200, Success(pm.GetContextBreakdown()))
}

func (s *Server) getProject(c *gin.Context) {
	res := make(map[string]any)
	s.mu.RLock()
	pm, exists := s.pms[c.Param("name")]
	s.mu.RUnlock()
	if !exists {
		c.JSON(404, Fail("project not found"))
		return
	}
	res["projectName"] = pm.GetProjectName()
	var cl []Container
	for _, co := range pm.GetContainerList() {
		cl = append(cl, Container{ContainerId: co.ContainerId, ContainerIP: co.ContainerIP, Image: co.Image, WebPort: co.WebPort})
	}
	res["containerList"] = cl
	// 兼容前端：也提供更直观的字段名
	res["containers"] = cl
	res["eventLog"] = pm.GetEvent(50)
	res["vulnList"] = map[string]any{
		"exploitIdeaList":  pm.GetExploitIdeaList(),
		"exploitChainList": pm.GetExploitChainList(),
	}
	// 详情页常用字段：避免进入详情页后再挨个请求
	res["exploitIdeas"] = pm.GetExploitIdeaList()
	res["exploitChains"] = pm.GetExploitChainList()
	res["agentRuns"] = pm.GetAgentRuntimeList()
	res["digitalHumans"] = pm.GetDigitalHumanRoster()
	report := make(map[string]string)
	for k, v := range pm.GetReportList() {
		report[k] = filepath.Base(v)
	}
	res["reportList"] = report
	res["reports"] = report
	res["status"] = pm.GetStatus()
	// 给 UI 更容易判断的运行状态 — "决策结束" 也算运行中（项目未真正结束）
	res["isRunning"] = pm.GetStatus() == "正在运行" || pm.GetStatus() == "决策结束"
	res["brainFinished"] = pm.GetBrainFinished()
	res["startTime"] = pm.GetStartTime()
	res["endTime"] = pm.GetEndTime()
	res["EnvInfo"] = pm.GetEnvInfo()
	res["envInfo"] = pm.GetEnvInfo()
	c.JSON(200, Success(res))
}
func (s *Server) createProject(c *gin.Context) {
	// Check required config before creating project.
	missing := misc.CheckRequiredConfig()
	misc.Debug("createProject: CheckRequiredConfig => missing=%v", missing)
	if len(missing) > 0 {
		c.JSON(400, Fail("请先在「设置」中配置必填项: "+strings.Join(missing, ", ")))
		return
	}

	projectName := c.DefaultQuery("projectName", "project-"+uuid.New().String())
	matched, err := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, projectName)
	if err != nil {
		c.JSON(400, Fail("ProjectName只允许^[a-zA-Z0-9_-]+$"))
		return
	}
	if !matched {
		c.JSON(400, Fail("ProjectName只允许^[a-zA-Z0-9_-]+$"))
		return
	}

	taskContent := c.PostForm("taskContent")
	sourceType := c.DefaultPostForm("source_type", "file") // file | git | url

	absPath, err := filepath.Abs(misc.GetDataDir())
	if err != nil {
		c.JSON(500, Fail("获取DataDir"))
		return
	}
	tempDir := filepath.Join(absPath, "temp", uuid.New().String())
	defer os.RemoveAll(tempDir)

	switch sourceType {
	case "file":
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(400, Fail("请上传源码压缩包: "+err.Error()))
			return
		}
		ext := filepath.Ext(file.Filename)
		tempFile, err := os.CreateTemp("", "upload-*"+ext)
		if err != nil {
			c.JSON(500, Fail("创建临时文件失败"))
			return
		}
		defer os.Remove(tempFile.Name())
		defer tempFile.Close()
		src, err := file.Open()
		if err != nil {
			c.JSON(500, Fail("打开上传文件失败"))
			return
		}
		defer src.Close()
		if _, err := io.Copy(tempFile, src); err != nil {
			c.JSON(500, Fail("保存上传文件失败"))
			return
		}
		if err := UncompressFile(tempFile.Name(), tempDir); err != nil {
			c.JSON(500, Fail("解压失败: "+err.Error()))
			return
		}

	case "git":
		gitURL := c.PostForm("git_url")
		if gitURL == "" {
			c.JSON(400, Fail("请提供 Git 仓库地址"))
			return
		}
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			c.JSON(500, Fail("创建临时目录失败"))
			return
		}
		cmd := exec.CommandContext(c.Request.Context(), "git", "clone", "--depth", "1", gitURL, tempDir)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		output, err := cmd.CombinedOutput()
		if err != nil {
			c.JSON(500, Fail("Git clone 失败: "+err.Error()+"\n"+string(output)))
			return
		}

	case "url":
		fileURL := c.PostForm("file_url")
		if fileURL == "" {
			c.JSON(400, Fail("请提供压缩包下载链接"))
			return
		}
		resp, err := http.Get(fileURL)
		if err != nil {
			c.JSON(500, Fail("下载失败: "+err.Error()))
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			c.JSON(500, Fail("下载失败: HTTP "+strconv.Itoa(resp.StatusCode)))
			return
		}
		// Determine extension from URL or Content-Type
		ext := filepath.Ext(fileURL)
		if ext == "" || len(ext) > 6 {
			ext = ".zip"
		}
		tempFile, err := os.CreateTemp("", "download-*"+ext)
		if err != nil {
			c.JSON(500, Fail("创建临时文件失败"))
			return
		}
		defer os.Remove(tempFile.Name())
		defer tempFile.Close()
		if _, err := io.Copy(tempFile, resp.Body); err != nil {
			c.JSON(500, Fail("保存下载文件失败"))
			return
		}
		if err := UncompressFile(tempFile.Name(), tempDir); err != nil {
			c.JSON(500, Fail("解压失败: "+err.Error()))
			return
		}

	default:
		c.JSON(400, Fail("不支持的 source_type: "+sourceType))
		return
	}

	// Flatten single top-level directory wrapper.
	// Archives often contain a single root folder (e.g., project-name/src/...).
	// Unwrap it so source files sit directly in tempDir.
	if entries, err := os.ReadDir(tempDir); err == nil {
		dirs, files := 0, 0
		var singleDir string
		for _, e := range entries {
			if e.IsDir() {
				// Skip .git directory when counting
				if e.Name() == ".git" {
					continue
				}
				dirs++
				singleDir = e.Name()
			} else {
				files++
			}
		}
		if dirs == 1 && files == 0 {
			nested := filepath.Join(tempDir, singleDir)
			if subEntries, err := os.ReadDir(nested); err == nil {
				for _, se := range subEntries {
					src := filepath.Join(nested, se.Name())
					dst := filepath.Join(tempDir, se.Name())
					_ = os.Rename(src, dst)
				}
				_ = os.Remove(nested)
			}
		}
	}

	projectConfig := ProjectManager.ProjectConfig{ProjectName: projectName, SourceCodeDir: tempDir, MsgChan: s.msgChan, TaskContent: taskContent}
	pm, err := ProjectManager.NewProjectManager(projectConfig)
	if err != nil {
		c.JSON(500, Fail(err.Error()))
		return
	}
	s.mu.Lock()
	s.pms[projectName] = pm
	s.mu.Unlock()
	s.SaveProjectManagerToFile()
	c.JSON(200, Success("成功新建项目"))
}

func (s *Server) startProject(c *gin.Context) {
	s.mu.RLock()
	pm, exists := s.pms[c.Param("name")]
	s.mu.RUnlock()
	if !exists {
		c.JSON(404, Fail("project not found"))
		return
	}
	go pm.StartTask()
	c.JSON(200, Success("项目开始运行"))
}
func (s *Server) cancelProject(c *gin.Context) {
	s.mu.RLock()
	pm, exists := s.pms[c.Param("name")]
	s.mu.RUnlock()
	if !exists {
		c.JSON(404, Fail("project not found"))
		return
	}
	go pm.StopTask()
	c.JSON(200, Success("项目已经取消"))
}
func (s *Server) getEnvInfo(c *gin.Context) {
	s.mu.RLock()
	pm, exists := s.pms[c.Param("name")]
	s.mu.RUnlock()
	if !exists {
		c.JSON(404, Fail("project not found"))
		return
	}
	c.JSON(200, Success(pm.GetEnvInfo()))
}

func (s *Server) delProject(c *gin.Context) {
	s.mu.RLock()
	pm, exists := s.pms[c.Param("name")]
	s.mu.RUnlock()
	if !exists {
		c.JSON(404, Fail("project not found"))
		return
	}
	pm.StopTask()
	pm.RemoveDockerAll()
	s.mu.Lock()
	delete(s.pms, pm.GetProjectName())
	s.mu.Unlock()
	s.SaveProjectManagerToFile()
	c.JSON(200, Success("删除成功"))
}

func (s *Server) listModels(c *gin.Context) {
	baseURL := c.Query("base_url")
	apiKey := c.Query("api_key")
	if baseURL == "" || apiKey == "" {
		c.JSON(400, Fail("base_url and api_key are required"))
		return
	}
	// Trim trailing slash
	baseURL = regexp.MustCompile(`/+$`).ReplaceAllString(baseURL, "")
	url := baseURL + "/models"

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		c.JSON(500, Fail("request error: "+err.Error()))
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", misc.GetConfigValueDefault("main_setting", "USER_AGENT", "AIxVuln"))

	resp, err := client.Do(req)
	if err != nil {
		c.JSON(502, Fail("upstream error: "+err.Error()))
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// Try to parse OpenAI-style response: { "data": [ { "id": "model-name" }, ... ] }
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	var models []string
	if err := json.Unmarshal(body, &parsed); err == nil && len(parsed.Data) > 0 {
		for _, m := range parsed.Data {
			if m.ID != "" {
				models = append(models, m.ID)
			}
		}
	}
	c.JSON(200, Success(models))
}

func (s *Server) getConfig(c *gin.Context) {
	c.JSON(200, Success(misc.GetAllConfig()))
}

func (s *Server) setConfig(c *gin.Context) {
	var body map[string]map[string]string
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, Fail("invalid JSON: "+err.Error()))
		return
	}
	if err := misc.SetAllConfig(body); err != nil {
		c.JSON(500, Fail("save config failed: "+err.Error()))
		return
	}
	misc.ReloadDebugFlag()
	c.JSON(200, Success("配置已保存，部分配置需要重启后生效"))
}

func (s *Server) getDigitalHumans(c *gin.Context) {
	c.JSON(200, Success(misc.GetDigitalHumansList()))
}

func (s *Server) saveDigitalHuman(c *gin.Context) {
	var body misc.DigitalHumanRow
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, Fail("invalid JSON: "+err.Error()))
		return
	}
	if body.ID == "" || body.AgentType == "" || body.PersonaName == "" {
		c.JSON(400, Fail("id, agent_type, persona_name are required"))
		return
	}
	if err := misc.SaveDigitalHuman(body); err != nil {
		c.JSON(500, Fail("save failed: "+err.Error()))
		return
	}
	c.JSON(200, Success("保存成功，重启项目后生效"))
}

func (s *Server) deleteDigitalHuman(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(400, Fail("id is required"))
		return
	}
	if err := misc.DeleteDigitalHuman(id); err != nil {
		c.JSON(500, Fail("delete failed: "+err.Error()))
		return
	}
	c.JSON(200, Success("删除成功，重启项目后生效"))
}

// initStatus returns the current initialization state (no auth required).
func initStatusHandler(c *gin.Context) {
	needsUser := !misc.HasAnyUser()

	// Check Docker images.
	images := map[string]bool{"aisandbox": false, "java_env": false}
	cmd := exec.Command("docker", "images", "--format", "{{.Repository}}")
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			name := strings.TrimSpace(line)
			switch name {
			case "aisandbox", "aixvuln/aisandbox":
				images["aisandbox"] = true
			case "java_env", "aixvuln/java_env":
				images["java_env"] = true
			}
		}
	}

	c.JSON(200, gin.H{
		"needs_user":    needsUser,
		"docker_images": images,
		"initialized":   !needsUser,
	})
}

// initSetup handles the first-run setup (no auth required, only works if no user exists).
func initSetupHandler(c *gin.Context) {
	if misc.HasAnyUser() {
		c.JSON(400, gin.H{"success": false, "error": "系统已初始化，无法重复设置"})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Username) == "" || len(req.Password) < 6 {
		c.JSON(400, gin.H{"success": false, "error": "用户名不能为空，密码至少6位"})
		return
	}
	if err := misc.CreateUser(strings.TrimSpace(req.Username), req.Password); err != nil {
		c.JSON(500, gin.H{"success": false, "error": "创建用户失败: " + err.Error()})
		return
	}
	token := generateToken(req.Username)
	c.JSON(200, gin.H{"success": true, "token": token})
}

// dockerAuthCheck validates token for docker endpoints.
// During init (no users exist) it is unauthenticated; after init it requires a valid token.
func dockerAuthCheck(c *gin.Context) bool {
	if misc.HasAnyUser() {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			c.AbortWithStatusJSON(401, gin.H{"success": false, "error": "unauthorized"})
			return false
		}
		if _, err := validateToken(strings.TrimPrefix(auth, "Bearer ")); err != nil {
			c.AbortWithStatusJSON(401, gin.H{"success": false, "error": "unauthorized: " + err.Error()})
			return false
		}
	}
	return true
}

// streamCmdOutput runs a command and streams its combined stdout/stderr via SSE.
// Each line is sent as a "data: {json}\n\n" event.
// Final event is either {"done":true} or {"done":true,"error":"..."}.
func streamCmdOutput(c *gin.Context, cmd *exec.Cmd) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout // merge stderr into stdout

	if err := cmd.Start(); err != nil {
		return err
	}

	// Set SSE headers.
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
	for scanner.Scan() {
		line := scanner.Text()
		evt, _ := json.Marshal(map[string]interface{}{"line": line})
		fmt.Fprintf(c.Writer, "data: %s\n\n", evt)
		c.Writer.Flush()
	}

	cmdErr := cmd.Wait()
	if cmdErr != nil {
		evt, _ := json.Marshal(map[string]interface{}{"done": true, "error": cmdErr.Error()})
		fmt.Fprintf(c.Writer, "data: %s\n\n", evt)
	} else {
		evt, _ := json.Marshal(map[string]interface{}{"done": true})
		fmt.Fprintf(c.Writer, "data: %s\n\n", evt)
	}
	c.Writer.Flush()
	return nil
}

// dockerBuild triggers a docker build for the given image name (SSE streaming output).
func dockerBuildHandler(c *gin.Context) {
	if !dockerAuthCheck(c) {
		return
	}
	imageName := c.Param("name")
	if imageName != "aisandbox" && imageName != "java_env" {
		c.JSON(400, gin.H{"success": false, "error": "不支持的镜像: " + imageName})
		return
	}
	contextPath, err := misc.GetDockerfilePath(imageName)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": "释放 Dockerfile 失败: " + err.Error()})
		return
	}
	dockerfilePath := filepath.Join(contextPath, "Dockerfile")
	cmd := exec.Command("docker", "build", "-t", imageName, "-f", dockerfilePath, contextPath)
	if err := streamCmdOutput(c, cmd); err != nil {
		c.JSON(500, gin.H{"success": false, "error": "构建启动失败: " + err.Error()})
	}
}

// dockerPull pulls a pre-built image from a registry and tags it with the local name (SSE streaming output).
// Accepts optional ?registry= query parameter to specify a custom registry prefix.
func dockerPullHandler(c *gin.Context) {
	if !dockerAuthCheck(c) {
		return
	}
	imageName := c.Param("name")
	allowedImages := map[string]bool{"aisandbox": true, "java_env": true}
	if !allowedImages[imageName] {
		c.JSON(400, gin.H{"success": false, "error": "不支持的镜像: " + imageName})
		return
	}
	// Determine remote image path: custom registry or default Docker Hub namespace.
	registry := strings.TrimSpace(c.Query("registry"))
	var remoteImage string
	if registry != "" {
		registry = strings.TrimRight(registry, "/")
		remoteImage = registry + "/" + imageName
	} else {
		remoteImage = "aixvuln/" + imageName
	}
	// Pull from registry (streamed).
	cmd := exec.Command("docker", "pull", remoteImage)
	if err := streamCmdOutput(c, cmd); err != nil {
		c.JSON(500, gin.H{"success": false, "error": "拉取启动失败: " + err.Error()})
		return
	}
	// Tag as local name so the system can find it.
	tagCmd := exec.Command("docker", "tag", remoteImage, imageName)
	tagOutput, err := tagCmd.CombinedOutput()
	if err != nil {
		evt, _ := json.Marshal(map[string]interface{}{"done": true, "error": "标记镜像失败: " + err.Error(), "output": string(tagOutput)})
		fmt.Fprintf(c.Writer, "data: %s\n\n", evt)
		c.Writer.Flush()
	}
}

func (s *Server) uploadAvatar(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, Fail("请上传头像文件: "+err.Error()))
		return
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" && ext != ".webp" {
		c.JSON(400, Fail("仅支持 png/jpg/jpeg/gif/webp 格式"))
		return
	}
	avatarDir := filepath.Join(misc.GetDataDir(), ".avatars")
	_ = os.MkdirAll(avatarDir, 0755)
	filename := uuid.New().String() + ext
	dst := filepath.Join(avatarDir, filename)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.JSON(500, Fail("保存头像失败: "+err.Error()))
		return
	}
	c.JSON(200, Success(filename))
}

func serveAvatar(c *gin.Context) {
	name := c.Param("name")
	if strings.Contains(name, "..") || strings.Contains(name, "/") {
		c.JSON(400, gin.H{"error": "invalid filename"})
		return
	}
	avatarDir := filepath.Join(misc.GetDataDir(), ".avatars")
	filePath := filepath.Join(avatarDir, name)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.Status(404)
		return
	}
	c.File(filePath)
}

func (s *Server) getReportTemplates(c *gin.Context) {
	c.JSON(200, Success(misc.GetAllReportTemplates()))
}

func (s *Server) setReportTemplate(c *gin.Context) {
	var body map[string]string
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, Fail("invalid JSON: "+err.Error()))
		return
	}
	for name, content := range body {
		if err := misc.SetReportTemplate(name, content); err != nil {
			c.JSON(500, Fail("保存模板失败: "+err.Error()))
			return
		}
	}
	c.JSON(200, Success("报告模板已保存"))
}
