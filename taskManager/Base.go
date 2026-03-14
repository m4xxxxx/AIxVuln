package taskManager

import (
	"AIxVuln/dockerManager"
	"fmt"
)

var projects *ProjectS

type ContainerInfo struct {
	Type        string   `json:"type"` //Create OR Remove
	ContainerId string   `json:"containerId"`
	ContainerIP string   `json:"containerIP"`
	Image       string   `json:"image"`
	WebPort     []string `json:"webPort"`
}

type ProjectInfo struct {
	ProjectName   string                 `json:"projectName"`
	SourceCodeDir string                 `json:"source_code_dir"`
	TaskContent   string                 `json:"taskContent"`
	SandboxID     string                 `json:"sandbox_container_id,omitempty"`
	StartTime     string                 `json:"start_time"`
	EndTime       string                 `json:"end_time"`
	ContainerList []ContainerInfo        `json:"containerList"`
	VulnList      []Vuln                 `json:"vuln_list"`
	ExploitIdeas  []*ExploitIdea         `json:"exploit_ideas,omitempty"`
	ExploitChains []*ExploitChain        `json:"exploit_chains,omitempty"`
	TokenUsage    TokenUsageSnapshot     `json:"token_usage,omitempty"`
	EventList     []string               `json:"event_list"`
	EnvInfo       map[string]interface{} `json:"envInfo"`
	ProjectDir    string                 `json:"projectDir"`
	ReportList    map[string]string      `json:"report_list"`
}

type AgentTokenUsageSnapshot struct {
	Label            string `json:"label"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

type TokenUsageSnapshot struct {
	PromptTokens     int64                     `json:"prompt_tokens"`
	CompletionTokens int64                     `json:"completion_tokens"`
	TotalTokens      int64                     `json:"total_tokens"`
	Agents           []AgentTokenUsageSnapshot `json:"agents,omitempty"`
}

type ProjectS struct {
	sandboxList    map[string]*dockerManager.Sandbox
	serviceManager map[string]*dockerManager.ServiceManager
	dockerManager  map[string]*dockerManager.DockerManager
	goroutineChan  map[string]chan func()
}

func NewProjectS() *ProjectS {
	return &ProjectS{sandboxList: make(map[string]*dockerManager.Sandbox), serviceManager: make(map[string]*dockerManager.ServiceManager), dockerManager: make(map[string]*dockerManager.DockerManager), goroutineChan: make(map[string]chan func())}
}
func (p *ProjectS) SetGoroutineChan(projectName string, fc chan func()) {
	p.goroutineChan[projectName] = fc
}

func (p *ProjectS) SetSandbox(projectName string, sandbox *dockerManager.Sandbox) {
	p.sandboxList[projectName] = sandbox
}
func (p *ProjectS) GetSandbox(projectName string) (*dockerManager.Sandbox, error) {
	s, ok := p.sandboxList[projectName]
	if !ok {
		return nil, fmt.Errorf("project %s not found", projectName)
	}
	return s, nil
}

func (p *ProjectS) RemoveSandbox(projectName string) {
	delete(p.sandboxList, projectName)
}

func (p *ProjectS) SetServiceManager(projectName string, sm *dockerManager.ServiceManager) {
	p.serviceManager[projectName] = sm
}
func (p *ProjectS) GetServiceManager(projectName string) (*dockerManager.ServiceManager, error) {
	s, ok := p.serviceManager[projectName]
	if !ok {
		return nil, fmt.Errorf("project %s not found", projectName)
	}
	return s, nil
}

func (p *ProjectS) SetDockerManager(projectName string, dm *dockerManager.DockerManager) {
	p.dockerManager[projectName] = dm
}
func (p *ProjectS) GetDockerManager(projectName string) (*dockerManager.DockerManager, error) {
	s, ok := p.dockerManager[projectName]
	if !ok {
		return nil, fmt.Errorf("project %s not found", projectName)
	}
	return s, nil
}

func init() {
	projects = NewProjectS()
}

func SetSandbox(projectName string, sandbox *dockerManager.Sandbox) {
	projects.sandboxList[projectName] = sandbox
}
func SetServiceManager(projectName string, sm *dockerManager.ServiceManager) {
	projects.serviceManager[projectName] = sm
}
func SetDockerManager(projectName string, dm *dockerManager.DockerManager) {
	projects.dockerManager[projectName] = dm
}
func GetSandbox(projectName string) (*dockerManager.Sandbox, error) {
	return projects.GetSandbox(projectName)
}
func RemoveSandbox(projectName string) {
	projects.RemoveSandbox(projectName)
}
func GetServiceManager(projectName string) (*dockerManager.ServiceManager, error) {
	return projects.GetServiceManager(projectName)
}
func GetDockerManager(projectName string) (*dockerManager.DockerManager, error) {
	return projects.GetDockerManager(projectName)
}
func SetGoroutineChan(projectName string, fc chan func()) {
	projects.goroutineChan[projectName] = fc
}
func GetGoroutineChan(projectName string) chan func() {
	return projects.goroutineChan[projectName]
}
