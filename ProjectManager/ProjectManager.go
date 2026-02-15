package ProjectManager

import (
	"AIxVuln/DecisionBrain"
	"AIxVuln/agents"
	"AIxVuln/dockerManager"
	"AIxVuln/misc"
	"AIxVuln/taskManager"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

type ProjectManager struct {
	mu             sync.RWMutex
	wg             *sync.WaitGroup
	tasks          []*taskManager.Task
	projectName    string
	taskContent    string
	agentGroupList AgentGroupList
	goroutine      chan func()
	sourceCodeDir  string
	projectDir     string
	containerList  []taskManager.ContainerInfo
	vulnList       []taskManager.Vuln //整个项目的漏洞汇总
	eventList      []string
	agentRespList  map[string]*agents.StartResp
	reportList     map[string]string
	status         string
	startTime      string
	endTime        string
	stopChan       chan struct{}
	ctx            context.Context
	cancelFunc     context.CancelFunc
	isStopping     bool
	isRunning      bool
	msgChan        chan string
	envInfo        map[string]interface{}
	dm             *dockerManager.DockerManager
	decisionBrain  *DecisionBrain.DecisionBrain
	brainRestart   chan struct{}
}

type ProjectConfig struct {
	ProjectName   string
	SourceCodeDir string
	MsgChan       chan string
	TaskContent   string
}

type AgentGroup struct {
	Name      string // Agent组名称
	AgentList []*AgentOne
	Runed     bool // 是否已经运行完成
}

type AgentOne struct {
	Agent         agents.Agent
	InheritEnv    string // 从某个Agent继承关键信息，填写AgentId
	InheritVuln   string // 从某个Agent继承Vuln信息，填写AgentId
	InheritMemory string // 从某个Agent继承记忆体信息，填写AgentId
}

type AgentGroupList struct {
	AgentList []*AgentGroup
}

func (a *AgentGroupList) putAgentGroup(ag *AgentGroup) {
	a.AgentList = append(a.AgentList, ag)
}
func (a *AgentGroupList) nextAgentGroup() *AgentGroup {
	for _, one := range a.AgentList {
		if one.Runed == false {
			return one
		}
	}
	return nil
}

func NewProjectManager(project ProjectConfig) (*ProjectManager, error) {
	matched, err := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, project.ProjectName)
	if err != nil {
		return nil, fmt.Errorf("ProjectName只允许^[a-zA-Z0-9_-]+$")
	}
	if !matched {
		return nil, fmt.Errorf("ProjectName只允许^[a-zA-Z0-9_-]+$")
	}
	absDataDir, _ := filepath.Abs(misc.GetDataDir())
	projectDir := filepath.Join(absDataDir, "projects", project.ProjectName)
	err = os.MkdirAll(projectDir, 0755)
	if err != nil {
		return nil, err
	}
	sourceCodeDir := filepath.Join(projectDir, "sourceCodeDir")
	err = misc.CopyDir(project.SourceCodeDir, sourceCodeDir)
	if err != nil {
		return nil, err
	}
	pm := ProjectManager{
		wg:            &sync.WaitGroup{},
		projectName:   project.ProjectName,
		taskContent:   project.TaskContent,
		goroutine:     make(chan func(), 100),
		sourceCodeDir: sourceCodeDir,
		projectDir:    projectDir,
		agentRespList: make(map[string]*agents.StartResp),
		reportList:    make(map[string]string),
		status:        "未运行",
		stopChan:      make(chan struct{}, 1), // 带缓冲的通道
		isStopping:    false,
		isRunning:     false,
		brainRestart:  make(chan struct{}, 1),
		msgChan:       project.MsgChan,
		dm:            dockerManager.NewDockerManager(),
	}
	pm.ctx, pm.cancelFunc = context.WithCancel(context.Background())
	pm.goroutineOps()
	taskManager.SetDockerManager(pm.projectName, pm.dm)
	sandbox := dockerManager.NewSandbox(pm.dm, sourceCodeDir)
	taskManager.SetSandbox(pm.projectName, sandbox)
	sm := dockerManager.NewServiceManager(sourceCodeDir, pm.dm)
	taskManager.SetServiceManager(pm.projectName, sm)
	taskManager.SetGoroutineChan(pm.projectName, pm.goroutine)
	pm.decisionBrain = DecisionBrain.NewDecisionBrain(project.ProjectName, project.TaskContent, project.MsgChan)
	pm.decisionBrain.SetStatusHandler(func(status string) {
		pm.status = status
	})
	as, _ := agents.GetAgentDescription()
	for _, v := range as {
		pm.decisionBrain.RegisterAgent(v.Name, v.NewFunc)
	}
	// Load digital human profiles from SQLite.
	dhMap := misc.GetAllDigitalHumans()
	pool := make(map[string][]agents.AgentProfile)
	for agentType, rows := range dhMap {
		profiles := make([]agents.AgentProfile, 0, len(rows))
		for _, r := range rows {
			profiles = append(profiles, agents.AgentProfile{
				DigitalHumanID: r.ID,
				PersonaName:    r.PersonaName,
				Gender:         r.Gender,
				AvatarFile:     r.AvatarFile,
				Personality:    r.Personality,
				Age:            r.Age,
				ExtraSysPrompt: r.ExtraSysPrompt,
			})
		}
		pool[agentType] = profiles
	}
	pm.decisionBrain.InitDigitalHumanPool(pool)
	return &pm, nil
}

func (pm *ProjectManager) GetDockerManager() *dockerManager.DockerManager {
	return pm.dm
}

func (pm *ProjectManager) SetMsgChan(msgChan chan string) {
	pm.msgChan = msgChan
}

func (pm *ProjectManager) GetProjectName() string {
	return pm.projectName
}

func (pm *ProjectManager) GetTaskContent() string {
	return pm.taskContent
}
func (pm *ProjectManager) GetStatus() string {
	return pm.status
}
func (pm *ProjectManager) GetStartTime() string {
	return pm.startTime
}
func (pm *ProjectManager) GetEndTime() string {
	return pm.endTime
}
func (pm *ProjectManager) SetStatus(status string) {
	pm.status = status
}
func (pm *ProjectManager) GetSourceCodeDir() string {
	return pm.sourceCodeDir
}
func (pm *ProjectManager) GetProjectDir() string {
	return pm.projectDir
}

func (pm *ProjectManager) goroutineOps() {
	go func() {
		for f := range pm.goroutine {
			go func() {
				pm.wg.Add(1)
				f()
				pm.wg.Done()
			}()
		}
	}()
}

func (pm *ProjectManager) StartTask() {
	if pm.taskContent == "" {
		pm.status = "任务内容为空"
		return
	}
	if pm.isRunning {
		return
	}
	pm.status = "正在运行"
	pm.isRunning = true
	pm.isStopping = false
	pm.startTime = time.Now().Format("2006-01-02 15:04:05")
	defer func() {
		pm.status = "运行结束"
		pm.isRunning = false
		pm.endTime = time.Now().Format("2006-01-02 15:04:05")
	}()

	// ---- Phase 0: Run ProjectOverviewAgent to scan the project before the brain starts. ----
	pm.status = "项目概览分析中"
	misc.Debug("StartTask: 开始项目概览分析")
	pm.decisionBrain.EmitBrainMessage("正在进行项目概览分析，识别编程语言、框架和技术栈...")
	overviewSummary := pm.runProjectOverview()
	if overviewSummary != "" {
		pm.decisionBrain.SetProjectOverview(overviewSummary)
		misc.Debug("StartTask: 项目概览完成，已注入决策大脑")
		pm.decisionBrain.EmitBrainMessage("项目概览分析完成:\n" + overviewSummary)
	} else {
		misc.Debug("StartTask: 项目概览未产生结果，跳过注入")
		pm.decisionBrain.EmitBrainMessage("项目概览分析未产生结果，跳过")
	}
	pm.status = "正在运行"

	for {
		pm.wg.Add(1)
		go func() {
			defer pm.wg.Done()
			pm.decisionBrain.Start()
		}()
		pm.wg.Wait()

		// If brain entered "决策结束" state, stay alive and wait for user to
		// either chat (which restarts the brain) or click "结束项目".
		if !pm.decisionBrain.IsBrainFinished() {
			break
		}
		// Block here until TeamChat calls RestartAfterFinished + signals, or StopTask is called.
		select {
		case <-pm.brainRestart:
			// User sent a chat message — loop back and run Start() again.
			continue
		case <-pm.ctx.Done():
			// StopTask was called — exit.
			return
		}
	}
}

// runProjectOverview runs the ProjectOverviewAgent synchronously and returns the summary.
func (pm *ProjectManager) runProjectOverview() string {
	task := taskManager.NewTask(pm.projectName)
	task.SetEventHandler(func(agentName string, event string, eventType int) {
		// Minimal event handler — overview events are not critical.
		misc.Debug("[ProjectOverview] %s: %s", agentName, event)
	})
	agent, err := agents.NewProjectOverviewAgent(task, "{}")
	if err != nil {
		misc.Debug("runProjectOverview: 创建 agent 失败: %s", err.Error())
		return ""
	}
	ctx, cancel := context.WithTimeout(pm.ctx, 3*time.Minute)
	defer cancel()
	// Start the persistent StartTask loop in a goroutine, then send the task via AssignTask.
	doneCh := make(chan *agents.StartResp, 1)
	go agent.StartTask(ctx)
	agent.AssignTask(agents.TaskAssignment{
		ArgsJson: "{}",
		DoneCb: func(r *agents.StartResp) {
			doneCh <- r
		},
	})
	select {
	case resp := <-doneCh:
		if resp.Err != nil {
			misc.Debug("runProjectOverview: agent 执行失败: %s", resp.Err.Error())
			return ""
		}
		return resp.Summary
	case <-ctx.Done():
		misc.Debug("runProjectOverview: context 超时")
		return ""
	}
}

func (pm *ProjectManager) StopTask() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	// 防止重复调用
	if pm.isStopping {
		return
	}
	pm.status = "正在停止"
	pm.isStopping = true
	if pm.decisionBrain != nil {
		pm.decisionBrain.Stop()
	}
	// 关键：调用 context 的取消函数
	if pm.cancelFunc != nil {
		pm.cancelFunc()
	}
}
func (pm *ProjectManager) GetContainerList() []taskManager.ContainerInfo {
	if pm.decisionBrain != nil {
		cl := pm.decisionBrain.GetContainerList()
		if len(cl) > 0 {
			out := make([]taskManager.ContainerInfo, 0, len(cl))
			for _, c := range cl {
				if c == nil {
					continue
				}
				out = append(out, *c)
			}
			return out
		}
	}
	return pm.containerList
}
func (pm *ProjectManager) GetEvent(count int) []string {
	if pm.decisionBrain != nil {
		e := pm.decisionBrain.GetEvent(count)
		if len(e) > 0 {
			return e
		}
	}
	l := len(pm.eventList)
	if l < count || l == 0 {
		return pm.eventList
	}
	return pm.eventList[l-count:]
}

func (pm *ProjectManager) GetBrainFeed(count int) []map[string]interface{} {
	if pm.decisionBrain == nil {
		return nil
	}
	return pm.decisionBrain.GetBrainFeed(count)
}

func (pm *ProjectManager) GetAgentFeed(agentID string, count int) []map[string]interface{} {
	if pm.decisionBrain == nil {
		return nil
	}
	return pm.decisionBrain.GetAgentFeed(agentID, count)
}

func (pm *ProjectManager) GetVulnList() []taskManager.Vuln {
	return pm.vulnList
}
func (pm *ProjectManager) SetVulns(vuln []taskManager.Vuln) {
	pm.vulnList = vuln
}
func (pm *ProjectManager) SetEvent(event []string) {
	pm.eventList = event
}
func (pm *ProjectManager) SetProjectDir(dir string) {
	pm.projectDir = dir
}
func (pm *ProjectManager) SetStartTime(t string) {
	pm.startTime = t
}
func (pm *ProjectManager) SetEndTime(t string) {
	pm.endTime = t
}
func (pm *ProjectManager) GetReportList() map[string]string {
	if pm.decisionBrain != nil {
		rl := pm.decisionBrain.GetReportList()
		if len(rl) > 0 {
			m := make(map[string]string)
			for i, item := range rl {
				rid, _ := item["rid"]
				vid, ok1 := item["vid"]
				path, ok2 := item["path"]
				if !ok1 || !ok2 {
					continue
				}
				key := rid
				if key == "" {
					key = vid
				}
				if key == "" {
					key = filepath.Base(path)
					if key == "" {
						key = fmt.Sprintf("report-%d", i)
					}
				}
				if _, exists := m[key]; exists {
					key = fmt.Sprintf("%s-%d", key, i)
				}
				m[key] = path
			}
			return m
		}
	}
	return pm.reportList
}
func (pm *ProjectManager) GetEnvInfo() map[string]interface{} {
	if pm.decisionBrain != nil {
		env := pm.decisionBrain.GetEnvInfo()
		if len(env) > 0 {
			return env
		}
	}
	return pm.envInfo
}

func (pm *ProjectManager) GetExploitIdeaList() []*taskManager.ExploitIdea {
	if pm.decisionBrain == nil {
		return nil
	}
	return pm.decisionBrain.GetExploitIdeaList()
}

func (pm *ProjectManager) GetExploitChainList() []*taskManager.ExploitChain {
	if pm.decisionBrain == nil {
		return nil
	}
	return pm.decisionBrain.GetExploitChainList()
}

func (pm *ProjectManager) GetAgentRuntimeList() []map[string]interface{} {
	if pm.decisionBrain == nil {
		return nil
	}
	return pm.decisionBrain.GetAgentRuntimeList()
}

func (pm *ProjectManager) GetDigitalHumanRoster() map[string]interface{} {
	if pm.decisionBrain == nil {
		return nil
	}
	return pm.decisionBrain.GetDigitalHumanRoster()
}

func (pm *ProjectManager) GetBrainFinished() bool {
	if pm.decisionBrain == nil {
		return false
	}
	return pm.decisionBrain.IsBrainFinished()
}

func (pm *ProjectManager) TeamChat(msg string) string {
	if pm.decisionBrain == nil {
		return "project not started"
	}
	// If brain has finished and user sends a new message, restart the brain loop.
	if pm.decisionBrain.IsBrainFinished() {
		pm.decisionBrain.RestartAfterFinished()
		pm.status = "正在运行"
		// Signal the StartTask loop to restart the brain.
		select {
		case pm.brainRestart <- struct{}{}:
		default:
		}
	}
	return pm.decisionBrain.TeamChat(msg, "用户")
}

func (pm *ProjectManager) AppendChatMessage(msg DecisionBrain.ChatMessage) {
	if pm.decisionBrain != nil {
		pm.decisionBrain.AppendChatMessage(msg)
	}
}

func (pm *ProjectManager) GetChatMessages() []DecisionBrain.ChatMessage {
	if pm.decisionBrain == nil {
		return nil
	}
	return pm.decisionBrain.GetChatMessages()
}

func (pm *ProjectManager) GetTokenUsage() map[string]interface{} {
	if pm.decisionBrain == nil {
		return map[string]interface{}{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}
	}
	return pm.decisionBrain.GetTokenUsage()
}

func (pm *ProjectManager) GetContextBreakdown() interface{} {
	if pm.decisionBrain == nil {
		return map[string]interface{}{"sections": []interface{}{}, "total": 0, "max_context": 0}
	}
	return pm.decisionBrain.GetContextBreakdown()
}

func (pm *ProjectManager) SetEnvInfo(env map[string]interface{}) {
	pm.envInfo = env
}

func (pm *ProjectManager) SetContainer(containerList []taskManager.ContainerInfo) {
	pm.containerList = containerList
}

func (pm *ProjectManager) SetReport(reportList map[string]string) {
	pm.reportList = reportList
}

func (pm *ProjectManager) RemoveDockerAll() {
	if pm.dm == nil {
		return
	}
	// Remove project sandbox container (aisandbox)
	if sb, err := taskManager.GetSandbox(pm.projectName); err == nil && sb != nil {
		if sb.ContainerId != "" {
			misc.Debug("RemoveDockerAll: 删除 sandbox 容器 %s", sb.ContainerId)
			_ = pm.dm.DockerRemove(sb.ContainerId)
		}
		taskManager.RemoveSandbox(pm.projectName)
	}
	// Remove containers tracked by the DecisionBrain (runtime source of truth).
	if pm.decisionBrain != nil {
		for _, c := range pm.decisionBrain.GetContainerList() {
			if c == nil || c.ContainerId == "" {
				continue
			}
			misc.Debug("RemoveDockerAll: 删除容器 %s", c.ContainerId)
			_ = pm.dm.DockerRemove(c.ContainerId)
		}
	}
	// Also remove any containers in the local list (e.g. restored from persistence).
	for _, c := range pm.containerList {
		if c.ContainerId == "" {
			continue
		}
		_ = pm.dm.DockerRemove(c.ContainerId)
	}
}
