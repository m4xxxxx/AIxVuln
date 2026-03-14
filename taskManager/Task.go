package taskManager

import (
	"AIxVuln/dockerManager"
	"AIxVuln/llm"
	"AIxVuln/misc"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

type Task struct {
	sm                          *dockerManager.ServiceManager
	taskId                      string
	envInfo                     map[string]interface{}
	projectDir                  string
	taskDir                     string
	memory                      llm.Memory
	taskList                    []map[string]string
	agentName                   string
	projectName                 string
	goroutineChan               chan func()
	eventHandler                func(string, string, int)
	agentFeedHandler            func(string, string, map[string]interface{})
	reportHandler               func(string, string)
	envInfoHandle               func(map[string]interface{})
	candidateExploitIdeaHandler func(*ExploitIdea) error
	ExploitIdeaHandler          func(string, string, string, string) error
	ExploitChainHandler         func(string, string, string, string) error
	exploitIdeaGetter           func(string) (*ExploitIdea, error)
	exploitChainGetter          func(string) (*ExploitChain, error)
	addTaskHandle               func(*Task)
	//TaskDataHandle              func(ReportWritingRequest)
	currentVulnId        string
	Init                 bool
	guidanceHandler      func(string, string) string
	exploitIdeaQuota     int // per-task max submissions (0 = unlimited)
	exploitIdeaSubmitted int // per-task submission counter
}
type ReportWritingRequest struct {
	RequestType    string // verifier or analyzer
	HistoryMemory  []llm.Message
	SourceCodePath string
	Evidence       string
	POC            string
	Sandbox        *dockerManager.Sandbox
	EnvInfo        map[string]interface{}
	VulnId         string
	Candidate      string // analyzer 才需要
}

func NewTask(projectName string) *Task {
	uuidTmp, err := uuid.NewUUID()
	if err != nil {
		panic(err)
	}
	taskId := uuidTmp.String()
	projectDir := filepath.Join(misc.GetDataDir(), "projects", projectName)
	taskDir := filepath.Join(projectDir, "tasks", taskId)
	absTaskDir, err := filepath.Abs(taskDir)
	if err != nil {
		panic(err)
	}
	err = misc.CreateDir(absTaskDir)
	if err != nil {
		panic(err)
	}
	err = misc.CreateDir(filepath.Join(absTaskDir, "vuln"))
	if err != nil {
		panic(err)
	}
	sm, err := GetServiceManager(projectName)
	if err != nil {
		panic(err)
	}
	return &Task{sm: sm, taskId: taskId, taskDir: absTaskDir, goroutineChan: GetGoroutineChan(projectName), projectName: projectName, projectDir: projectDir}
}

func (task *Task) SetExploitIdeaQuota(n int) {
	task.exploitIdeaQuota = n
}
func (task *Task) GetExploitIdeaQuota() int {
	return task.exploitIdeaQuota
}
func (task *Task) GetExploitIdeaSubmitted() int {
	return task.exploitIdeaSubmitted
}

// ResetExploitIdeaSubmitted clears per-task submission usage.
func (task *Task) ResetExploitIdeaSubmitted() {
	task.exploitIdeaSubmitted = 0
}

// IncrementExploitIdeaSubmitted increments the counter and returns the new value.
func (task *Task) IncrementExploitIdeaSubmitted() int {
	task.exploitIdeaSubmitted++
	return task.exploitIdeaSubmitted
}

func (task *Task) SetGuidanceHandler(fc func(string, string) string) {
	task.guidanceHandler = fc
}
func (task *Task) GetGuidanceHandler() func(string, string) string {
	return task.guidanceHandler
}

func (task *Task) SetCandidateExploitIdeaHandler(fc func(*ExploitIdea) error) {
	task.candidateExploitIdeaHandler = fc
}
func (task *Task) GetCandidateExploitIdeaHandler() func(*ExploitIdea) error {
	return task.candidateExploitIdeaHandler
}
func (task *Task) SetExploitIdeaHandler(fc func(string, string, string, string) error) {
	task.ExploitIdeaHandler = fc
}
func (task *Task) GetExploitIdeaHandler() func(string, string, string, string) error {
	return task.ExploitIdeaHandler
}

func (task *Task) SetExploitChainHandler(fc func(string, string, string, string) error) {
	task.ExploitChainHandler = fc
}
func (task *Task) GetExploitChainHandler() func(string, string, string, string) error {
	return task.ExploitChainHandler
}

func (task *Task) SetExploitIdeaGetter(fc func(string) (*ExploitIdea, error)) {
	task.exploitIdeaGetter = fc
}
func (task *Task) GetExploitIdeaGetter() func(string) (*ExploitIdea, error) {
	return task.exploitIdeaGetter
}
func (task *Task) SetExploitChainGetter(fc func(string) (*ExploitChain, error)) {
	task.exploitChainGetter = fc
}
func (task *Task) GetExploitChainGetter() func(string) (*ExploitChain, error) {
	return task.exploitChainGetter
}

//	func (task *Task) SetTaskDataHandler(fc func(ReportWritingRequest)) {
//		task.TaskDataHandle = fc
//	}
//
//	func (task *Task) GetTaskDataHandle() func(ReportWritingRequest) {
//		return task.TaskDataHandle
//	}
func (task *Task) GetCurrVulnId() string {
	return task.currentVulnId
}
func (task *Task) GetTaskId() string {
	return task.taskId
}
func (task *Task) SetCurrVulnId(id string) {
	task.currentVulnId = id
}

func (task *Task) GetProjectDir() string {
	return task.projectDir
}
func (task *Task) GetProjectName() string {
	return task.projectName
}
func (task *Task) SetTaskList(x []map[string]string) {
	task.taskList = x
	task.memory.SetTaskList(&llm.TaskListX{TaskList: task.GetTaskList(), ContextId: task.taskId})
}
func (task *Task) SetEventHandler(handler func(string, string, int)) {
	task.eventHandler = handler
}
func (task *Task) GetEventHandler() func(string, string, int) {
	return task.eventHandler
}

func (task *Task) SetAgentFeedHandler(handler func(string, string, map[string]interface{})) {
	task.agentFeedHandler = handler
}
func (task *Task) GetAgentFeedHandler() func(string, string, map[string]interface{}) {
	return task.agentFeedHandler
}
func (task *Task) EmitAgentFeed(agentID string, kind string, data map[string]interface{}) {
	if task.agentFeedHandler == nil {
		return
	}
	task.agentFeedHandler(agentID, kind, data)
}
func (task *Task) SetReportHandler(handler func(string, string)) {
	task.reportHandler = handler
}
func (task *Task) GetReportHandler() func(string, string) {
	return task.reportHandler
}

func (task *Task) GetAddTaskHandler() func(*Task) {
	return task.addTaskHandle
}
func (task *Task) GetTaskList() []map[string]string {
	return task.taskList
}
func (task *Task) GetEnvInfo() map[string]interface{} {
	return task.envInfo
}
func (task *Task) GetTaskDir() string {
	return task.taskDir
}
func (task *Task) GetSm() *dockerManager.ServiceManager {
	return task.sm
}
func (task *Task) GetSourceCodePath() string {
	return task.sm.GetSourceCodePath()
}
func (task *Task) SetMemory(memory llm.Memory) {
	task.memory = memory
	if memory.GetType() == "SharedContext" {
		cm := llm.NewContextManager()
		cm.SetEventHandler(task.GetEventHandler())
		task.memory.AddContextManager(task.taskId, cm)
	}
	task.memory.SetTaskList(&llm.TaskListX{TaskList: task.GetTaskList(), ContextId: task.taskId})
}
func (task *Task) SetAgentName(name string) {
	task.agentName = name
}

func (task *Task) SetEnvInfo(envInfo map[string]interface{}) {
	task.envInfo = envInfo
	task.memory.AddKeyMessage(&llm.EnvMessageX{Key: "WebEnvInfo", Content: envInfo, AppendEnv: false})
}

func (task *Task) SaveEnvInfo(envInfo map[string]interface{}) error {
	if task.memory == nil {
		misc.Warn(task.projectName+"-"+task.agentName, "当前任务未设置memory 将不支持环境关键信息记录", task.eventHandler)
	} else {
		task.memory.AddKeyMessage(&llm.EnvMessageX{Key: "WebEnvInfo", Content: envInfo, AppendEnv: false})
		task.envInfoHandle(envInfo)
	}
	task.envInfo = envInfo
	envPath := task.projectDir + "/env.json"
	f, err := os.OpenFile(envPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	js, err := json.Marshal(task.envInfo)
	if err != nil {
		return err
	}
	_, err = f.Write(js)
	if err != nil {
		return err
	}
	defer f.Close()
	return nil
}
func (task *Task) GetMemory() llm.Memory {
	return task.memory
}
func (task *Task) AddKeyMessage(key string, msg any, appendEnv bool) {
	if task.memory == nil {
		misc.Warn(task.projectName+"-"+task.agentName, "当前任务未设置memory 将不支持环境关键信息记录", task.eventHandler)
	} else {
		task.memory.AddKeyMessage(&llm.EnvMessageX{Key: key, Content: msg, AppendEnv: appendEnv})
	}
}

func (task *Task) EventLog(msg string) error {
	filename := task.taskDir + "/" + task.agentName + "-event.log"
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logEntry := fmt.Sprintf("[%s] %s\n", timestamp, msg)
	_, err = file.WriteString(logEntry)
	return err
}

func (task *Task) SaveVuln(id string, markdown string, poc string) error {
	filename := task.taskDir + "/vulns/" + id + ".md"
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	markdown = markdown + "\n\n# POC\n```python\n" + poc + "\n```\n"
	file.WriteString(markdown)
	return nil
}
func (task *Task) AddGoroutine(f func()) {
	task.goroutineChan <- f
}
func (task *Task) GetGoroutineChan() chan func() {
	return task.goroutineChan
}
func (task *Task) SetEnvInfoHandler(f func(map[string]interface{})) {
	task.envInfoHandle = f
}
