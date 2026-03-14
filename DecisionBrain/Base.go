package DecisionBrain

import (
	"AIxVuln/agents"
	"AIxVuln/llm"
	"AIxVuln/misc"
	"AIxVuln/taskManager"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

func (db *DecisionBrain) startVerifiedReportForExploitChain(ec *taskManager.ExploitChain) {
	if ec == nil {
		return
	}
	args := map[string]string{"reportType": "verifier", "exploit_chain_id": ec.ExploitChainId}
	argsJson, _ := json.Marshal(args)
	_ = db.startAgent("Agent-Report-ReportCommonAgent", string(argsJson))
}

var auditPrompt = `You are an evidence reviewer. You need to review the provided runtime verification evidence for either an 'ExploitIdea' or an 'ExploitChain'. It will be explicitly stated whether it is an 'ExploitIdea' or an 'ExploitChain'. If it is an 'ExploitIdea', you will be provided with the condition, harm, and evidence. If it is an 'ExploitChain', you will be provided with the evidence. This evidence may contain claims that the conditions are met, but you must strictly determine whether the evidence meets any one of the following criteria:
Conclusive HTTP Response: Contains clear indications that the exploit idea or chain was successfully executed.
Exploitation Under Specified Conditions: The harm can be achieved when the provided condition is met. (Applies only to 'ExploitIdea')
Conclusive Exploit-Related Logs: For example, logs showing errors caused by SQL injection attempts.
POC Execution Result: Contains definitive proof of successful exploitation from running a Proof of Concept.
Other Evidence of Successful Exploitation: Such as confirmed delayed responses in SQL time-based blind injection.
Important: If you find the evidence passes the review, reply directly with the four letters true. If it does not pass the review, reply with the reasons why you think it fails and how it can be improved before resubmitting.
`
var guidancePromptTemplate = `你是安全审计团队的决策大脑（高级专家顾问）。一位数字人（智能助手）在执行任务时遇到了困难，正在向你请求指导。

## 提问者信息
%s

## 重要约束（请基于这些约束给出可行的建议）
1. 所有数字人都运行在隔离环境中，**没有宿主机访问权限**，只能通过提供的工具操作。
2. 代码分析数字人（Analyze）只有源码读取能力，不能执行命令、不能操作容器、不能访问运行环境。
3. 漏洞验证数字人（Verifier）可以在已搭建的目标环境中执行命令和代码，但不能修改源码、不能搭建环境。
4. 运维数字人（Ops）负责环境搭建和维护，可以操作 Docker 容器，但不负责漏洞分析。
5. 报告数字人（Report）只负责撰写报告，能力有限，主要读取源码和已有的漏洞数据。
%s

## 回答要求
- 用中文回答
- 给出具体、可操作的解决方案，而不是泛泛的建议
- 考虑提问者可用的工具，只建议其能力范围内的操作
- 如果问题超出提问者的能力范围，明确指出应该由哪类数字人来处理
- 简洁明了，不超过 500 字`

type DecisionBrain struct {
	memory            *BrainMemory
	agentHandler      map[string]func(task *taskManager.Task, args string) (agents.Agent, error)
	projectName       string
	memoryFilePath    string
	store             *ExploitStore
	exploitIdeaList   []*taskManager.ExploitIdea
	exploitChainList  []*taskManager.ExploitChain
	eventList         []string
	brainFeed         []map[string]interface{}
	agentFeed         map[string][]map[string]interface{}
	containerInfoList []string
	containerList     []*taskManager.ContainerInfo
	reportList        []map[string]string
	envInfo           map[string]interface{}
	ctxList           []context.Context
	cancelList        []context.CancelFunc
	ctx               context.Context
	cancelFunc        context.CancelFunc
	model             string
	runAgentList      []*AgentRuntime
	wg                sync.WaitGroup
	mu                sync.Mutex
	feedMu            sync.Mutex
	lastMsgHash       string
	notify            chan bool
	done              chan struct{}
	webOutputChan     chan string
	envBuildCond      *sync.Cond // 在环境没搭建完成的情况下阻塞
	poolMu            sync.Mutex
	digitalHumanPool  map[string][]digitalHumanEntry
	digitalHumanBusy  map[string]digitalHumanBusyInfo
	digitalHumanQueue map[string][]digitalHumanQueuedReq
	personaMemory     map[string]string
	lastSummary       map[string]string // keyed by DigitalHumanID
	lastTask          map[string]string // keyed by DigitalHumanID
	lastArgs          map[string]string // keyed by DigitalHumanID — original startAgent args
	chatFilePath      string
	chatMessages      []ChatMessage
	chatMu            sync.Mutex
	lastChatPersistAt time.Time
	chatPersistTimer  *time.Timer
	brainFinished     bool
	statusHandler     func(status string) // callback to notify ProjectManager of status changes
}

type ChatMessage struct {
	Role        string `json:"role"` // "user" or "system"
	Text        string `json:"text"`
	Ts          string `json:"ts"`
	PersonaName string `json:"persona_name,omitempty"`
	AvatarFile  string `json:"avatar_file,omitempty"`
}

const (
	maxDecisionEventList  = 5000
	maxDecisionChatRecord = 3000
	chatPersistDebounce   = 2 * time.Second
)

// digitalHumanEntry holds a persistent agent instance bound to a digital human profile.
type digitalHumanEntry struct {
	Profile agents.AgentProfile
	Agent   agents.Agent
}

type digitalHumanBusyInfo struct {
	AgentToolName string
	Profile       agents.AgentProfile
	Agent         agents.Agent
}

type digitalHumanQueuedReq struct {
	AgentToolName string
	Args          string
}

// trySendWS writes to webOutputChan with a timeout to prevent permanent blocking
// when the WebSocket consumer is slow or disconnected.
func (db *DecisionBrain) trySendWS(msg string) {
	if db.webOutputChan == nil {
		return
	}
	select {
	case db.webOutputChan <- msg:
	case <-time.After(5 * time.Second):
		misc.Debug("trySendWS: dropped message (channel full for 5s): %.100s", msg)
	}
}

func NewDecisionBrain(projectName string, taskContent string, webOutputChan chan string) *DecisionBrain {
	memory := NewBrainMemory()
	projectDir := filepath.Join(misc.GetDataDir(), "projects", projectName)
	memoryDir := filepath.Join(projectDir, "memory")
	_ = os.MkdirAll(memoryDir, 0755)
	memoryFile := filepath.Join(memoryDir, "brain.json")
	if _, err := os.Stat(memoryFile); err == nil {
		_ = memory.LoadMemoryFromFile(memoryFile)
	}
	memory.SetTaskContent(taskContent)
	model := misc.GetConfigValueDefault("decision", "MODEL", "")
	if model == "" {
		model = misc.GetConfigValueDefault("main_setting", "MODEL", "")
	}
	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	ctx, cancel := context.WithCancel(context.Background())
	chatFile := filepath.Join(memoryDir, "chat.json")
	var chatMessages []ChatMessage
	if data, err := os.ReadFile(chatFile); err == nil {
		_ = json.Unmarshal(data, &chatMessages)
	}
	if chatMessages == nil {
		chatMessages = make([]ChatMessage, 0)
	}
	if len(chatMessages) > maxDecisionChatRecord {
		chatMessages = append([]ChatMessage(nil), chatMessages[len(chatMessages)-maxDecisionChatRecord:]...)
	}
	db := &DecisionBrain{memory: memory,
		agentHandler:      make(map[string]func(*taskManager.Task, string) (agents.Agent, error)),
		exploitIdeaList:   make([]*taskManager.ExploitIdea, 0),
		projectName:       projectName,
		memoryFilePath:    memoryFile,
		chatFilePath:      chatFile,
		chatMessages:      chatMessages,
		reportList:        make([]map[string]string, 0),
		envInfo:           make(map[string]interface{}),
		ctx:               ctx,
		cancelFunc:        cancel,
		cancelList:        make([]context.CancelFunc, 0),
		digitalHumanPool:  make(map[string][]digitalHumanEntry),
		digitalHumanBusy:  make(map[string]digitalHumanBusyInfo),
		digitalHumanQueue: make(map[string][]digitalHumanQueuedReq),
		personaMemory:     make(map[string]string),
		lastSummary:       make(map[string]string),
		lastTask:          make(map[string]string),
		lastArgs:          make(map[string]string),
		model:             model,
		runAgentList:      make([]*AgentRuntime, 0),
		wg:                sync.WaitGroup{},
		mu:                sync.Mutex{},
		notify:            make(chan bool, 100),
		done:              make(chan struct{}),
		webOutputChan:     webOutputChan,
		exploitChainList:  make([]*taskManager.ExploitChain, 0),
		eventList:         make([]string, 0),
		brainFeed:         make([]map[string]interface{}, 0),
		agentFeed:         make(map[string][]map[string]interface{}),
		envBuildCond:      cond,
	}
	store, err := NewExploitStore(projectName)
	if err != nil {
		misc.Warn("decision", "init sqlite store failed: "+err.Error(), db.SubmitEventHandler)
	} else {
		db.store = store
	}
	dm, err := taskManager.GetDockerManager(projectName)
	if err != nil {
		panic(err)
	}
	dm.SetEventHandler(db.SubmitContainerEventHandler)
	return db
}

func (db *DecisionBrain) personaMemoryKey(agentToolName string, profile agents.AgentProfile) string {
	return agentToolName + "|" + strings.TrimSpace(profile.PersonaName)
}

func (db *DecisionBrain) getPersonaMemory(agentToolName string, profile agents.AgentProfile) string {
	key := db.personaMemoryKey(agentToolName, profile)
	db.poolMu.Lock()
	defer db.poolMu.Unlock()
	return db.personaMemory[key]
}

func (db *DecisionBrain) setPersonaMemory(agentToolName string, profile agents.AgentProfile, mem string) {
	key := db.personaMemoryKey(agentToolName, profile)
	db.poolMu.Lock()
	defer db.poolMu.Unlock()
	db.personaMemory[key] = mem
}

func (db *DecisionBrain) summarizePersonaMemory(agentToolName string, profile agents.AgentProfile, existing string, runTask string, runSummary string, ctxMsgs []llm.Message) string {
	// Keep only last N messages to control prompt size.
	maxMsgs := 30
	if len(ctxMsgs) > maxMsgs {
		ctxMsgs = ctxMsgs[len(ctxMsgs)-maxMsgs:]
	}
	trimmed := make([]map[string]string, 0, len(ctxMsgs))
	for _, m := range ctxMsgs {
		c := m.Content
		if len(c) > 800 {
			c = c[:800] + " ...[truncated]"
		}
		trimmed = append(trimmed, map[string]string{"role": m.Role, "content": c})
	}
	js, _ := json.Marshal(trimmed)

	sys := "You are a persona memory editor. Update the persona's long-term memory with new task experience.\n" +
		"Output MUST be concise plain text (Chinese).\n" +
		"Rules:\n" +
		"- Preserve: completed work, key findings, environment info (URLs/creds), important constraints, IDs, file paths, and decisions.\n" +
		"- Remove: verbose step-by-step logs, repeated tool outputs.\n" +
		"- Keep it compact: short sections + bullet points.\n" +
		"- Do NOT output code blocks."

	user := "Persona: " + profile.PersonaName + " (" + profile.Gender + ")\n" +
		"Agent tool: " + agentToolName + "\n\n" +
		"Existing persona memory (may be empty):\n" + existing + "\n\n" +
		"This run task:\n" + runTask + "\n\n" +
		"This run summary:\n" + runSummary + "\n\n" +
		"Recent chat context (JSON array of {role,content}):\n" + string(js)

	ms := []llm.Message{
		{Role: llm.RoleSystem, Content: sys},
		{Role: llm.RoleUser, Content: user},
	}

	ctx, cancel := context.WithTimeout(db.ctx, 5*60*time.Second)
	defer cancel()
	resp, err := llm.RequestLLM(llm.GetResponsesClient("decision", "main_setting"), ctx, db.model, ms, nil, db.projectName)
	if err != nil {
		return existing
	}
	if resp.Content == "" {
		return existing
	}
	return strings.TrimSpace(resp.Content)
}

func (db *DecisionBrain) InitDigitalHumanPool(pool map[string][]agents.AgentProfile) {
	db.poolMu.Lock()
	defer db.poolMu.Unlock()
	for agentToolName, profiles := range pool {
		entries := make([]digitalHumanEntry, 0, len(profiles))
		newFunc, exists := db.agentHandler[agentToolName]
		if !exists {
			misc.Debug("InitDigitalHumanPool: agentHandler not found for %s, skipping agent creation", agentToolName)
			for _, p := range profiles {
				entries = append(entries, digitalHumanEntry{Profile: p, Agent: nil})
			}
			db.digitalHumanPool[agentToolName] = entries
			continue
		}
		for _, p := range profiles {
			task := taskManager.NewTask(db.projectName)
			db.InitTask(task)
			agent, err := newFunc(task, "{}")
			if err != nil {
				misc.Debug("InitDigitalHumanPool: failed to create agent for %s/%s: %v", agentToolName, p.PersonaName, err)
				entries = append(entries, digitalHumanEntry{Profile: p, Agent: nil})
				continue
			}
			agent.SetProfile(p)
			// Per-digital-human system prompt customization.
			{
				basePersona := ""
				if strings.TrimSpace(p.PersonaName) != "" {
					basePersona += "你是数字人：" + strings.TrimSpace(p.PersonaName) + "。"
				}
				if strings.TrimSpace(p.Gender) != "" {
					basePersona += "性别：" + strings.TrimSpace(p.Gender) + "。"
				}
				if p.Age > 0 {
					basePersona += fmt.Sprintf("年龄：%d。", p.Age)
				}
				if strings.TrimSpace(p.Personality) != "" {
					basePersona += "性格：" + strings.TrimSpace(p.Personality) + "。"
				}
				extra := strings.TrimSpace(p.ExtraSysPrompt)
				full := strings.TrimSpace(basePersona)
				if extra != "" {
					if full != "" {
						full = full + "\n" + extra
					} else {
						full = extra
					}
				}
				if strings.TrimSpace(full) != "" {
					agent.GetMemory().SetExtraSystemPrompt(full, task.GetTaskId())
				}
			}
			agent.SetStateHandler(db.AgentStateUpdate)
			// Start the long-running StartTask goroutine.
			ctx, cancel := context.WithCancel(db.ctx)
			db.ctxList = append(db.ctxList, ctx)
			db.cancelList = append(db.cancelList, cancel)
			go func(a agents.Agent, c context.Context) {
				a.StartTask(c)
			}(agent, ctx)
			misc.Debug("InitDigitalHumanPool: created persistent agent %s for %s (dhID=%s)", agent.GetId(), p.PersonaName, p.DigitalHumanID)
			entries = append(entries, digitalHumanEntry{Profile: p, Agent: agent})
		}
		db.digitalHumanPool[agentToolName] = entries
		if _, ok := db.digitalHumanQueue[agentToolName]; !ok {
			db.digitalHumanQueue[agentToolName] = make([]digitalHumanQueuedReq, 0)
		}
	}
	db.updateDigitalHumanRosterMemoryLocked()
}

func (db *DecisionBrain) updateDigitalHumanRosterMemoryLocked() {
	// poolMu MUST be held.
	type rosterItem struct {
		DigitalHumanID string `json:"digital_human_id"`
		PersonaName    string `json:"persona_name"`
		Gender         string `json:"gender"`
		Personality    string `json:"personality"`
		Age            int    `json:"age"`
		AvatarFile     string `json:"avatar_file"`
		State          string `json:"state"` // idle|busy
		AgentID        string `json:"agent_id,omitempty"`
		QueueLength    int    `json:"queue_length"`
	}

	busyByTool := make(map[string][]rosterItem)
	for aid, b := range db.digitalHumanBusy {
		dhID := strings.TrimSpace(b.Profile.DigitalHumanID)
		busyByTool[b.AgentToolName] = append(busyByTool[b.AgentToolName], rosterItem{
			DigitalHumanID: dhID,
			PersonaName:    strings.TrimSpace(b.Profile.PersonaName),
			Gender:         strings.TrimSpace(b.Profile.Gender),
			Personality:    strings.TrimSpace(b.Profile.Personality),
			Age:            b.Profile.Age,
			AvatarFile:     strings.TrimSpace(b.Profile.AvatarFile),
			State:          "busy",
			AgentID:        aid,
			QueueLength:    len(db.digitalHumanQueue[b.AgentToolName]),
		})
	}

	roster := make(map[string][]rosterItem)
	for toolName, pool := range db.digitalHumanPool {
		items := make([]rosterItem, 0, len(pool)+len(busyByTool[toolName]))
		for _, entry := range pool {
			p := entry.Profile
			items = append(items, rosterItem{
				DigitalHumanID: strings.TrimSpace(p.DigitalHumanID),
				PersonaName:    strings.TrimSpace(p.PersonaName),
				Gender:         strings.TrimSpace(p.Gender),
				Personality:    strings.TrimSpace(p.Personality),
				Age:            p.Age,
				AvatarFile:     strings.TrimSpace(p.AvatarFile),
				State:          "idle",
				QueueLength:    len(db.digitalHumanQueue[toolName]),
			})
		}
		items = append(items, busyByTool[toolName]...)
		roster[toolName] = items
	}
	// Ensure tools that are only busy (no idle pool left) still show up.
	for toolName, items := range busyByTool {
		if _, ok := roster[toolName]; ok {
			continue
		}
		roster[toolName] = items
	}

	if db.memory != nil {
		db.memory.AddKeyMessage("DigitalHumanRoster", roster, false)
	}
	// Real-time panel update for UI
	if db.webOutputChan != nil {
		msg := WebMsg{Type: "DigitalHumanRosterUpdate", Data: roster, ProjectName: db.projectName}
		if b, err := json.Marshal(msg); err == nil {
			db.trySendWS(string(b))
		}
	}
}

func (db *DecisionBrain) GetDigitalHumanRoster() map[string]interface{} {
	db.poolMu.Lock()
	defer db.poolMu.Unlock()

	type rosterItem struct {
		DigitalHumanID string `json:"digital_human_id"`
		PersonaName    string `json:"persona_name"`
		Gender         string `json:"gender"`
		Personality    string `json:"personality"`
		Age            int    `json:"age"`
		AvatarFile     string `json:"avatar_file"`
		State          string `json:"state"` // idle|busy
		AgentID        string `json:"agent_id,omitempty"`
		QueueLength    int    `json:"queue_length"`
		LastSummary    string `json:"last_summary,omitempty"`
		LastTask       string `json:"last_task,omitempty"`
		ContextSize    int    `json:"context_size"`
		MaxContext     int    `json:"max_context"`
	}

	busyByTool := make(map[string][]rosterItem)
	for aid, b := range db.digitalHumanBusy {
		dhID := strings.TrimSpace(b.Profile.DigitalHumanID)
		var ctxSize, maxCtx int
		if b.Agent != nil {
			if mem := b.Agent.GetMemory(); mem != nil {
				ctxSize = mem.GetMsgSize(b.Agent.GetId())
				maxCtx = mem.GetMaxHistory()
			}
		}
		busyByTool[b.AgentToolName] = append(busyByTool[b.AgentToolName], rosterItem{
			DigitalHumanID: dhID,
			PersonaName:    strings.TrimSpace(b.Profile.PersonaName),
			Gender:         strings.TrimSpace(b.Profile.Gender),
			Personality:    strings.TrimSpace(b.Profile.Personality),
			Age:            b.Profile.Age,
			AvatarFile:     strings.TrimSpace(b.Profile.AvatarFile),
			State:          "busy",
			AgentID:        aid,
			QueueLength:    len(db.digitalHumanQueue[b.AgentToolName]),
			LastSummary:    db.lastSummary[dhID],
			LastTask:       db.lastTask[dhID],
			ContextSize:    ctxSize,
			MaxContext:     maxCtx,
		})
	}

	roster := make(map[string][]rosterItem)
	for toolName, pool := range db.digitalHumanPool {
		items := make([]rosterItem, 0, len(pool)+len(busyByTool[toolName]))
		for _, entry := range pool {
			p := entry.Profile
			dhID := strings.TrimSpace(p.DigitalHumanID)
			var ctxSize, maxCtx int
			if entry.Agent != nil {
				if mem := entry.Agent.GetMemory(); mem != nil {
					ctxSize = mem.GetMsgSize(entry.Agent.GetId())
					maxCtx = mem.GetMaxHistory()
				}
			}
			items = append(items, rosterItem{
				DigitalHumanID: dhID,
				PersonaName:    strings.TrimSpace(p.PersonaName),
				Gender:         strings.TrimSpace(p.Gender),
				Personality:    strings.TrimSpace(p.Personality),
				Age:            p.Age,
				AvatarFile:     strings.TrimSpace(p.AvatarFile),
				State:          "idle",
				QueueLength:    len(db.digitalHumanQueue[toolName]),
				LastSummary:    db.lastSummary[dhID],
				LastTask:       db.lastTask[dhID],
				ContextSize:    ctxSize,
				MaxContext:     maxCtx,
			})
		}
		items = append(items, busyByTool[toolName]...)
		roster[toolName] = items
	}
	for toolName, items := range busyByTool {
		if _, ok := roster[toolName]; ok {
			continue
		}
		roster[toolName] = items
	}

	out := make(map[string]interface{}, len(roster))
	for k, v := range roster {
		out[k] = v
	}
	return out
}

func (db *DecisionBrain) updateDigitalHumanRosterMemory() {
	db.poolMu.Lock()
	defer db.poolMu.Unlock()
	db.updateDigitalHumanRosterMemoryLocked()
	db.signal()
}

func (db *DecisionBrain) acquireDigitalHuman(agentToolName string) (digitalHumanEntry, bool) {
	db.poolMu.Lock()
	defer db.poolMu.Unlock()
	pool := db.digitalHumanPool[agentToolName]
	if len(pool) == 0 {
		return digitalHumanEntry{}, false
	}
	entry := pool[0]
	db.digitalHumanPool[agentToolName] = pool[1:]
	return entry, true
}

func (db *DecisionBrain) acquireDigitalHumanByID(agentToolName string, dhID string) (digitalHumanEntry, bool) {
	db.poolMu.Lock()
	defer db.poolMu.Unlock()
	pool := db.digitalHumanPool[agentToolName]
	for i, entry := range pool {
		if strings.TrimSpace(entry.Profile.DigitalHumanID) == dhID {
			db.digitalHumanPool[agentToolName] = append(pool[:i], pool[i+1:]...)
			misc.Debug("acquireDigitalHumanByID: 成功获取 personaName=%s, dhID=%s", entry.Profile.PersonaName, entry.Profile.DigitalHumanID)
			return entry, true
		}
	}
	misc.Debug("acquireDigitalHumanByID: 未找到 dhID=%s in pool for %s", dhID, agentToolName)
	return digitalHumanEntry{}, false
}

func (db *DecisionBrain) releaseDigitalHuman(agentID string) {
	db.poolMu.Lock()
	busy, ok := db.digitalHumanBusy[agentID]
	if !ok {
		db.poolMu.Unlock()
		return
	}
	delete(db.digitalHumanBusy, agentID)
	db.digitalHumanPool[busy.AgentToolName] = append(db.digitalHumanPool[busy.AgentToolName], digitalHumanEntry{Profile: busy.Profile, Agent: busy.Agent})

	q := db.digitalHumanQueue[busy.AgentToolName]
	if len(q) == 0 {
		db.updateDigitalHumanRosterMemoryLocked()
		db.poolMu.Unlock()
		return
	}
	// Dequeue one and try to start it.
	next := q[0]
	if len(q) == 1 {
		db.digitalHumanQueue[busy.AgentToolName] = nil
	} else {
		db.digitalHumanQueue[busy.AgentToolName] = q[1:]
	}
	db.updateDigitalHumanRosterMemoryLocked()
	db.poolMu.Unlock()

	go func() {
		startMsg := db.startAgent(next.AgentToolName, next.Args)
		// If this is a Verifier task that was queued, update the ExploitIdea/Chain
		// state from "等待验证" to "正在验证" now that the agent is actually running.
		if strings.Contains(next.AgentToolName, "Verifier") && strings.Contains(startMsg, "Agent ran successfully") {
			var argsMap map[string]interface{}
			if json.Unmarshal([]byte(next.Args), &argsMap) == nil {
				if eid, ok := argsMap["exploit_idea_id"]; ok && fmt.Sprint(eid) != "" {
					if ei, err := db.GetExploitIdeaById(fmt.Sprint(eid)); err == nil && ei.State == "等待验证" {
						ei.State = "正在验证"
						misc.Debug("releaseDigitalHuman: 排队任务启动成功，ExploitIdea %s 状态更新为正在验证", fmt.Sprint(eid))
						db.flushExploitIdeaList()
					}
				}
				if cid, ok := argsMap["exploit_chain_id"]; ok && fmt.Sprint(cid) != "" {
					if ec, err := db.GetExploitChainById(fmt.Sprint(cid)); err == nil && ec.State == "等待验证" {
						ec.State = "正在验证"
						misc.Debug("releaseDigitalHuman: 排队任务启动成功，ExploitChain %s 状态更新为正在验证", fmt.Sprint(cid))
						db.flushExploitChainList()
					}
				}
			}
		}
	}()
}

func (db *DecisionBrain) Stop() {
	select {
	case <-db.done:
		return
	default:
		close(db.done)
	}
	if db.cancelFunc != nil {
		db.cancelFunc()
	}
	for _, c := range db.cancelList {
		if c != nil {
			c()
		}
	}
	// Wake up any goroutines waiting for env build.
	if db.envBuildCond != nil {
		db.envBuildCond.L.Lock()
		db.envBuildCond.Broadcast()
		db.envBuildCond.L.Unlock()
	}
	if db.store != nil {
		_ = db.store.Close()
	}
	db.chatMu.Lock()
	if db.chatPersistTimer != nil {
		db.chatPersistTimer.Stop()
		db.chatPersistTimer = nil
	}
	db.saveChatMessagesLocked()
	db.lastChatPersistAt = time.Now()
	db.chatMu.Unlock()
}

func (db *DecisionBrain) signal() {
	select {
	case db.notify <- true:
	default:
	}
}

func (db *DecisionBrain) SetStatusHandler(h func(status string)) {
	db.statusHandler = h
}

// SetProjectOverview injects the project overview (language, framework, tech stack, etc.)
// into the brain's key messages so it is available from the very first decision.
func (db *DecisionBrain) SetProjectOverview(overview string) {
	db.memory.AddKeyMessage("ProjectOverview", overview, false)
}

func (db *DecisionBrain) IsBrainFinished() bool {
	return db.brainFinished
}

func (db *DecisionBrain) setBrainFinished(v bool) {
	db.brainFinished = v
	if db.statusHandler != nil {
		if v {
			db.statusHandler("决策结束")
		} else {
			db.statusHandler("正在运行")
		}
	}
}

func (db *DecisionBrain) RestartAfterFinished() {
	db.setBrainFinished(false)
	// Notify frontend that brain is running again.
	if db.webOutputChan != nil {
		statusMsg := WebMsg{Type: "BrainFinished", Data: map[string]interface{}{
			"brain_finished": false,
		}, ProjectName: db.projectName}
		if b, err := json.Marshal(statusMsg); err == nil {
			db.trySendWS(string(b))
		}
	}
}

func (db *DecisionBrain) hasRunningAgents() bool {
	return len(db.getActiveAgentIDs()) > 0
}

func (db *DecisionBrain) getActiveAgentIDs() []string {
	var ids []string
	for _, r := range db.runAgentList {
		if r == nil || r.agent == nil {
			continue
		}
		if r.done {
			continue
		}
		// Verifier agents may be queued waiting for env build and not yet "Running".
		// As long as the runtime is not done and the agent hasn't reached a terminal state,
		// we should treat it as active to prevent the decision brain from exiting early.
		st := strings.ToLower(strings.TrimSpace(r.agent.GetState()))
		if st != "done" && st != "completed" && st != "success" {
			ids = append(ids, r.agent.GetId())
		}
	}
	return ids
}

func (db *DecisionBrain) InitTask(task *taskManager.Task) {
	//task.SetTaskDataHandler(db.SubmitReportWritingRequestHandler)
	task.SetReportHandler(db.AddReport)
	task.SetEnvInfoHandler(db.SetEnvInfo)
	task.SetEventHandler(db.SubmitEventHandler)
	task.SetAgentFeedHandler(db.SubmitAgentFeedHandler)
	task.SetCandidateExploitIdeaHandler(db.SubmitCandidateExploitIdeaHandler)
	task.SetExploitIdeaHandler(db.SubmitExploitIdeaHandler)
	task.SetExploitChainHandler(db.SubmitExploitChainHandler)
	task.SetExploitIdeaGetter(db.GetExploitIdeaById)
	task.SetExploitChainGetter(db.GetExploitChainById)
	task.SetGuidanceHandler(func(source, question string) string {
		return db.Guidance(source, question)
	})
	m := llm.NewContextManager()
	m.SetEventHandler(db.SubmitEventHandler)
	task.SetMemory(m)
}
func (db *DecisionBrain) AddReport(vid string, path string) {
	rid := uuid.New().String()
	db.reportList = append(db.reportList, map[string]string{"rid": rid, "vid": vid, "path": path})
}
func (db *DecisionBrain) RegisterAgent(name string, fc func(*taskManager.Task, string) (agents.Agent, error)) {
	db.agentHandler[name] = fc
}

// 这个是候选idea提交通道（未经验证的）
func (db *DecisionBrain) SubmitCandidateExploitIdeaHandler(exploitIdea *taskManager.ExploitIdea) error {
	if exploitIdea == nil {
		return fmt.Errorf("exploitIdea is nil")
	}
	if strings.TrimSpace(exploitIdea.Harm) == "" {
		return fmt.Errorf("harm is required")
	}
	if strings.TrimSpace(exploitIdea.ExtendIdea) == "" {
		return fmt.Errorf("extendIdea is required")
	}
	if strings.TrimSpace(exploitIdea.Condition) == "" {
		return fmt.Errorf("condition is required")
	}
	if exploitIdea.ExploitPoint == nil {
		return fmt.Errorf("exploitPoint is required")
	}
	if strings.TrimSpace(exploitIdea.ExploitPoint.Title) == "" {
		return fmt.Errorf("exploitPoint.title is required")
	}
	if strings.TrimSpace(exploitIdea.ExploitPoint.RouteOrEndpoint) == "" {
		return fmt.Errorf("exploitPoint.route_or_endpoint is required")
	}
	if strings.TrimSpace(exploitIdea.ExploitPoint.File) == "" {
		return fmt.Errorf("exploitPoint.file is required")
	}
	if strings.TrimSpace(exploitIdea.ExploitPoint.FunctionOrMethod) == "" {
		return fmt.Errorf("exploitPoint.function_or_method is required")
	}
	if strings.TrimSpace(exploitIdea.ExploitPoint.Params) == "" {
		return fmt.Errorf("exploitPoint.params is required")
	}
	if strings.TrimSpace(exploitIdea.ExploitPoint.Type) == "" {
		return fmt.Errorf("exploitPoint.type is required")
	}
	if strings.TrimSpace(exploitIdea.ExploitPoint.PayloadIdea) == "" {
		return fmt.Errorf("exploitPoint.payload_idea is required")
	}
	if strings.TrimSpace(exploitIdea.ExploitPoint.ExpectedImpact) == "" {
		return fmt.Errorf("exploitPoint.expected_impact is required")
	}
	if strings.TrimSpace(exploitIdea.ExploitPoint.Confidence) == "" {
		return fmt.Errorf("exploitPoint.confidence is required")
	}
	index := 0
	for {
		id := fmt.Sprintf("E.%d", index)
		exists := false
		for _, v := range db.exploitIdeaList {
			if v.ExploitIdeaId == id {
				exists = true
				break
			}
		}
		if exists {
			index++
			continue
		}
		exploitIdea.ExploitIdeaId = id
		exploitIdea.State = "未验证"
		db.exploitIdeaList = append(db.exploitIdeaList, exploitIdea)
		db.flushExploitIdeaList()
		// Broadcast updated exploitIdea list to all running analysis agents so they avoid duplicate mining.
		db.broadcastExploitIdeaSummaryToAnalyzers()
		if err := db.VerifyExploitIdea(exploitIdea.ExploitIdeaId); err != nil {
			return err
		}
		return nil
	}
}

// buildExploitIdeaSummaryJSON returns a compact JSON summary of all discovered exploitIdeas.
func (db *DecisionBrain) buildExploitIdeaSummaryJSON() string {
	type ideaBrief struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Type  string `json:"type"`
		File  string `json:"file"`
		Route string `json:"route"`
		State string `json:"state"`
	}
	briefs := make([]ideaBrief, 0, len(db.exploitIdeaList))
	for _, ei := range db.exploitIdeaList {
		b := ideaBrief{ID: ei.ExploitIdeaId, State: ei.State}
		if ei.ExploitPoint != nil {
			b.Title = ei.ExploitPoint.Title
			b.Type = ei.ExploitPoint.Type
			b.File = ei.ExploitPoint.File
			b.Route = ei.ExploitPoint.RouteOrEndpoint
		}
		briefs = append(briefs, b)
	}
	js, err := json.Marshal(briefs)
	if err != nil {
		return "[]"
	}
	return string(js)
}

// injectExploitIdeaSummary injects the current exploitIdea summary into a single agent's keyMessage.
func (db *DecisionBrain) injectExploitIdeaSummary(a agents.Agent) {
	summary := db.buildExploitIdeaSummaryJSON()
	a.GetMemory().AddKeyMessage(&llm.EnvMessageX{
		Key:       "DiscoveredExploitIdeas",
		Content:   summary,
		AppendEnv: false,
	})
}

// broadcastExploitIdeaSummaryToAnalyzers builds a compact summary of all discovered
// exploitIdeas and injects it into every running analysis agent's keyMessage so they
// can see what has already been found and avoid duplicate mining.
func (db *DecisionBrain) broadcastExploitIdeaSummaryToAnalyzers() {
	summary := db.buildExploitIdeaSummaryJSON()

	// Find all running analysis agents and inject the summary.
	db.mu.Lock()
	var analyzeAgents []agents.Agent
	for _, ar := range db.runAgentList {
		if ar != nil && ar.agent != nil && !ar.done && strings.Contains(ar.agentToolName, "Analyze") {
			analyzeAgents = append(analyzeAgents, ar.agent)
		}
	}
	db.mu.Unlock()

	for _, a := range analyzeAgents {
		a.GetMemory().AddKeyMessage(&llm.EnvMessageX{
			Key:       "DiscoveredExploitIdeas",
			Content:   summary,
			AppendEnv: false,
		})
	}
}

// 这个是验证后idea提交通道，需要审核提交的证据是否幻觉
func (db *DecisionBrain) SubmitExploitIdeaHandler(exploitIdeaId string, exploitIdeaStatus string, evidence string, poc string) error {
	var exploitIdea *taskManager.ExploitIdea
	for _, exploitIdea1 := range db.exploitIdeaList {
		if exploitIdea1.ExploitIdeaId == exploitIdeaId {
			exploitIdea = exploitIdea1
			break
		}
	}
	if exploitIdea == nil {
		return fmt.Errorf("exploitIdeaId %s not exist", exploitIdeaId)
	}
	if exploitIdea.ExploitPoint == nil {
		return fmt.Errorf("exploitIdeaId %s exploitPoint is empty", exploitIdeaId)
	}
	exploitIdea.Evidence = evidence
	exploitIdea.Poc = poc
	if exploitIdeaStatus == "Completed" {
		e := db.auditEvidence(evidence, exploitIdea.Condition, exploitIdea.Harm)
		if e != nil {
			exploitIdea.State = "审核失败，正在整改"
			exploitIdea.ReviewReason = e.Error()
			db.flushExploitIdeaList()
			return e
		}
		exploitIdea.State = "可利用"
		if db.store != nil {
			_ = db.store.UpsertExploitableIdea(exploitIdea)
		}
	} else {
		exploitIdea.State = "验证失败"
	}
	db.flushExploitIdeaList()
	return nil
}

// 这个是验证后利用链提交通道，需要审核提交的证据是否幻觉
func (db *DecisionBrain) SubmitExploitChainHandler(exploitChainId string, exploitChainStatus string, evidence string, poc string) error {
	exploitChain, err := db.GetExploitChainById(exploitChainId)
	if err != nil {
		return err
	}
	exploitChain.Evidence = evidence
	exploitChain.Poc = poc
	if exploitChainStatus == "Completed" {
		e := db.auditEvidence(evidence, "", "")
		if e != nil {
			exploitChain.State = "审核失败，正在改进后重新提交"
			exploitChain.ReviewReason = e.Error()
			db.flushExploitChainList()
			return e
		}
		exploitChain.State = "可利用"
		if db.store != nil {
			_ = db.store.UpsertExploitableChain(exploitChain)
		}
		db.startVerifiedReportForExploitChain(exploitChain)
	} else {
		exploitChain.State = "验证失败"
	}
	db.flushExploitChainList()
	return nil
}

//func (db *DecisionBrain) SubmitReportWritingRequestHandler(request taskManager.ReportWritingRequest) {
//
//}

func (db *DecisionBrain) SubmitContainerEventHandler(infoJson string) {
	db.containerInfoList = append(db.containerInfoList, infoJson)
	// Avoid double-serialization: each element is already a JSON string,
	// so wrap them as json.RawMessage before marshaling the array.
	raw := make([]json.RawMessage, 0, len(db.containerInfoList))
	for _, s := range db.containerInfoList {
		raw = append(raw, json.RawMessage(s))
	}
	js, _ := json.Marshal(raw)
	db.memory.SetContainerListInfo(string(js))
	db.AddContainerInfo(infoJson)
}
func (db *DecisionBrain) AddContainerInfo(infoStr string) {
	var containerInfo taskManager.ContainerInfo
	err := json.Unmarshal([]byte(infoStr), &containerInfo)
	if err != nil {
		misc.Warn("容器事件管理", "格式不正确："+infoStr, db.SubmitEventHandler)
	}
	if containerInfo.Type == "Remove" {
		result := make([]*taskManager.ContainerInfo, 0, len(db.containerList))
		for _, container := range db.containerList {
			if container.ContainerId != containerInfo.ContainerId {
				result = append(result, container)
			}
		}
		db.containerList = result
		s := WebMsg{Type: "ContainerRemove", Data: map[string]interface{}{"containerId": containerInfo.ContainerId}, ProjectName: db.projectName}
		js, _ := json.Marshal(s)
		db.trySendWS(string(js))
		misc.Warn("容器事件", fmt.Sprintf("删除容器：%s", infoStr), db.SubmitEventHandler)
		return
	}

	// Create/Add: update container list (dedupe)
	if containerInfo.ContainerId != "" {
		exists := false
		for i := range db.containerList {
			c := db.containerList[i]
			if c != nil && c.ContainerId == containerInfo.ContainerId {
				// Update fields in case we get a second Create for same id
				*c = containerInfo
				exists = true
				break
			}
		}
		if !exists {
			ci := containerInfo
			db.containerList = append(db.containerList, &ci)
		}
	}

	s := WebMsg{Type: "ContainerAdd", Data: containerInfo, ProjectName: db.projectName}
	js, _ := json.Marshal(s)
	db.trySendWS(string(js))
	misc.Success("容器事件", fmt.Sprintf("新容器：%s", infoStr), db.SubmitEventHandler)
}

func (db *DecisionBrain) SubmitEventHandler(mod string, msg string, level int) {
	fmt.Println(mod, msg, level)
	timeStr := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s][%s]: %s", timeStr, mod, msg)
	db.feedMu.Lock()
	db.eventList = append(db.eventList, line)
	if len(db.eventList) > maxDecisionEventList {
		db.eventList = append([]string(nil), db.eventList[len(db.eventList)-maxDecisionEventList:]...)
	}
	db.feedMu.Unlock()
	s := WebMsg{Type: "string", Data: line, ProjectName: db.projectName}
	js, _ := json.Marshal(s)
	db.trySendWS(string(js))
}

func (db *DecisionBrain) flushExploitIdeaList() {
	type exploitIdeaPanelItem struct {
		ExploitIdeaId string                    `json:"exploitIdeaId"`
		Harm          string                    `json:"harm"`
		ExtendIdea    string                    `json:"extend_idea"`
		Condition     string                    `json:"condition"`
		State         string                    `json:"state"`
		ExploitPoint  *taskManager.ExploitPoint `json:"exploit_point"`
	}
	items := make([]exploitIdeaPanelItem, 0, len(db.exploitIdeaList))
	for _, e := range db.exploitIdeaList {
		if e == nil {
			continue
		}
		items = append(items, exploitIdeaPanelItem{
			ExploitIdeaId: e.ExploitIdeaId,
			Harm:          e.Harm,
			ExtendIdea:    e.ExtendIdea,
			Condition:     e.Condition,
			State:         e.State,
			ExploitPoint:  e.ExploitPoint,
		})
	}
	js, _ := json.Marshal(items)
	db.memory.SetExploitIdeaList(string(js))
	db.signal()

	// Real-time panel update for UI
	msg := WebMsg{Type: "ExploitIdeaList", Data: items, ProjectName: db.projectName}
	if b, err := json.Marshal(msg); err == nil {
		db.trySendWS(string(b))
	}
}

func (db *DecisionBrain) flushExploitChainList() {
	type exploitIdeaPanelItem struct {
		ExploitIdeaId string                    `json:"exploitIdeaId"`
		Harm          string                    `json:"harm"`
		ExtendIdea    string                    `json:"extend_idea"`
		Condition     string                    `json:"condition"`
		State         string                    `json:"state"`
		ExploitPoint  *taskManager.ExploitPoint `json:"exploit_point"`
	}
	type exploitChainPanelItem struct {
		ExploitIdea    []exploitIdeaPanelItem `json:"exploit_idea"`
		Idea           string                 `json:"idea"`
		State          string                 `json:"state"`
		ExploitChainId string                 `json:"exploit_chain_id"`
	}
	items := make([]exploitChainPanelItem, 0, len(db.exploitChainList))
	for _, ec := range db.exploitChainList {
		if ec == nil {
			continue
		}
		eideas := make([]exploitIdeaPanelItem, 0, len(ec.ExploitIdea))
		for _, e := range ec.ExploitIdea {
			if e == nil {
				continue
			}
			eideas = append(eideas, exploitIdeaPanelItem{
				ExploitIdeaId: e.ExploitIdeaId,
				Harm:          e.Harm,
				ExtendIdea:    e.ExtendIdea,
				Condition:     e.Condition,
				State:         e.State,
				ExploitPoint:  e.ExploitPoint,
			})
		}
		items = append(items, exploitChainPanelItem{
			ExploitIdea:    eideas,
			Idea:           ec.Idea,
			State:          ec.State,
			ExploitChainId: ec.ExploitChainId,
		})
	}
	js, _ := json.Marshal(items)
	db.memory.SetExploitChainList(string(js))
	db.signal()

	// Real-time panel update for UI
	msg := WebMsg{Type: "ExploitChainList", Data: items, ProjectName: db.projectName}
	if b, err := json.Marshal(msg); err == nil {
		db.trySendWS(string(b))
	}
}

func (db *DecisionBrain) startAgent(name string, args string) string {
	return db.startAgentWithFollowUp(name, args, "", "")
}

func (db *DecisionBrain) startAgentWithFollowUp(name string, args string, followUpUserMsg string, preferDHID string) string {
	_, exists := db.agentHandler[name]
	if !exists {
		misc.Debug("找不到Agent %s", name)
		return fmt.Errorf("Agent %s not found", name).Error()
	}
	var entry digitalHumanEntry
	var ok bool
	if preferDHID != "" {
		misc.Debug("startAgentWithFollowUp: 尝试按ID获取数字人 preferDHID=%s, toolName=%s", preferDHID, name)
		entry, ok = db.acquireDigitalHumanByID(name, preferDHID)
	}
	if !ok {
		misc.Debug("startAgentWithFollowUp: 按ID未获取到，使用默认获取 toolName=%s", name)
		entry, ok = db.acquireDigitalHuman(name)
	}
	if !ok {
		// No idle digital human for this agent tool, enqueue.
		db.poolMu.Lock()
		db.digitalHumanQueue[name] = append(db.digitalHumanQueue[name], digitalHumanQueuedReq{AgentToolName: name, Args: args})
		queuedLen := len(db.digitalHumanQueue[name])
		db.updateDigitalHumanRosterMemoryLocked()
		db.poolMu.Unlock()
		return fmt.Sprintf("No idle digital human for %s. Task queued. Queue length: %d", name, queuedLen)
	}
	profile := entry.Profile
	agent := entry.Agent
	if agent == nil {
		misc.Debug("startAgentWithFollowUp: agent is nil for %s/%s, returning entry to pool", name, profile.PersonaName)
		db.poolMu.Lock()
		db.digitalHumanPool[name] = append(db.digitalHumanPool[name], entry)
		db.updateDigitalHumanRosterMemoryLocked()
		db.poolMu.Unlock()
		return fmt.Sprintf("Agent instance not available for %s/%s", name, profile.PersonaName)
	}
	misc.Debug("startAgentWithFollowUp: 获取到持久化Agent personaName=%s, dhID=%s, agentId=%s", profile.PersonaName, profile.DigitalHumanID, agent.GetId())
	task := agent.GetTask()
	senderPersonaName := profile.PersonaName
	senderAvatarFile := profile.AvatarFile
	// Inject persona memory summary (if any) so this digital human remembers past work.
	if mem := db.getPersonaMemory(name, profile); strings.TrimSpace(mem) != "" {
		agent.GetMemory().AddKeyMessage(&llm.EnvMessageX{Key: "PersonaMemory", Content: mem, AppendEnv: false})
	}
	// Override ExploitIdeaHandler per-agent so audit failure generates chat messages.
	task.SetExploitIdeaHandler(func(exploitIdeaId, exploitIdeaStatus, evidence, poc string) error {
		err := db.SubmitExploitIdeaHandler(exploitIdeaId, exploitIdeaStatus, evidence, poc)
		if err != nil {
			// Digital human → Decision Brain: report audit failure
			dhMsg := fmt.Sprintf("@决策大脑 我提交的 ExploitIdea %s 证据审核未通过，原因：%s。我将根据审核意见整改后重新提交。", exploitIdeaId, err.Error())
			db.BroadcastChatMessage(ChatMessage{Role: "system", Text: dhMsg, Ts: time.Now().Format("15:04:05"), PersonaName: senderPersonaName, AvatarFile: senderAvatarFile})
			// Decision Brain → Digital human: instruct to fix
			brainMsg := fmt.Sprintf("@%s 收到，ExploitIdea %s 的证据审核未通过。请根据审核意见补充真实的运行时证据后重新提交，不要编造或臆测证据内容。", senderPersonaName, exploitIdeaId)
			db.BroadcastChatMessage(ChatMessage{Role: "system", Text: brainMsg, Ts: time.Now().Format("15:04:05"), PersonaName: "决策大脑", AvatarFile: "system.png"})
		}
		return err
	})
	// Override ExploitChainHandler per-agent so audit failure generates chat messages.
	task.SetExploitChainHandler(func(exploitChainId, exploitChainStatus, evidence, poc string) error {
		err := db.SubmitExploitChainHandler(exploitChainId, exploitChainStatus, evidence, poc)
		if err != nil {
			dhMsg := fmt.Sprintf("@决策大脑 我提交的 ExploitChain %s 证据审核未通过，原因：%s。我将根据审核意见整改后重新提交。", exploitChainId, err.Error())
			db.BroadcastChatMessage(ChatMessage{Role: "system", Text: dhMsg, Ts: time.Now().Format("15:04:05"), PersonaName: senderPersonaName, AvatarFile: senderAvatarFile})
			brainMsg := fmt.Sprintf("@%s 收到，ExploitChain %s 的证据审核未通过。请根据审核意见补充真实的运行时证据后重新提交，不要编造或臆测证据内容。", senderPersonaName, exploitChainId)
			db.BroadcastChatMessage(ChatMessage{Role: "system", Text: brainMsg, Ts: time.Now().Format("15:04:05"), PersonaName: "决策大脑", AvatarFile: "system.png"})
		}
		return err
	})
	// Override GuidanceHandler per-agent so we can show Q&A in chat with identity.
	task.SetGuidanceHandler(func(source, q string) string {
		qText := "@决策大脑 " + q
		db.AppendChatMessage(ChatMessage{Role: "system", Text: qText, Ts: time.Now().Format("15:04:05"), PersonaName: senderPersonaName, AvatarFile: senderAvatarFile})
		if db.webOutputChan != nil {
			qMsg := WebMsg{Type: "UserMessage", Data: map[string]interface{}{
				"persona_name": senderPersonaName,
				"avatar_file":  senderAvatarFile,
				"agent_id":     agent.GetId(),
				"message":      qText,
			}, ProjectName: db.projectName}
			if b, err := json.Marshal(qMsg); err == nil {
				db.trySendWS(string(b))
			}
		}
		answer := db.Guidance(source, q)
		aText := "@" + senderPersonaName + " " + answer
		db.AppendChatMessage(ChatMessage{Role: "system", Text: aText, Ts: time.Now().Format("15:04:05"), PersonaName: "决策大脑", AvatarFile: "system.png"})
		if db.webOutputChan != nil {
			aMsg := WebMsg{Type: "UserMessage", Data: map[string]interface{}{
				"persona_name": "决策大脑",
				"avatar_file":  "system.png",
				"agent_id":     "",
				"message":      aText,
			}, ProjectName: db.projectName}
			if b, err := json.Marshal(aMsg); err == nil {
				db.trySendWS(string(b))
			}
		}
		return answer
	})
	// Build a human-readable task description for the chat assignment message.
	var assignText string
	{
		var argsMap map[string]interface{}
		_ = json.Unmarshal([]byte(args), &argsMap)
		isReport := strings.Contains(name, "Report")
		if eid, ok := argsMap["exploit_idea_id"]; ok && fmt.Sprint(eid) != "" {
			if isReport {
				assignText = "@" + senderPersonaName + " 报告任务: 请为 ExploitIdea " + fmt.Sprint(eid) + " 撰写漏洞报告"
			} else {
				assignText = "@" + senderPersonaName + " 验证任务: 请验证 ExploitIdea " + fmt.Sprint(eid)
			}
		} else if cid, ok := argsMap["exploit_chain_id"]; ok && fmt.Sprint(cid) != "" {
			if isReport {
				assignText = "@" + senderPersonaName + " 报告任务: 请为 ExploitChain " + fmt.Sprint(cid) + " 撰写漏洞报告"
			} else {
				assignText = "@" + senderPersonaName + " 验证任务: 请验证 ExploitChain " + fmt.Sprint(cid)
			}
		} else {
			taskContent := ""
			if tc, ok := argsMap["task_content"]; ok {
				taskContent = strings.TrimSpace(fmt.Sprint(tc))
			}
			if taskContent == "" {
				taskContent = args
			}
			assignText = "@" + senderPersonaName + " " + taskContent
		}
	}
	isDeferred := strings.HasPrefix(name, "Agent-Verifier") || strings.HasPrefix(name, "Agent-Report")
	if !isDeferred {
		db.AppendChatMessage(ChatMessage{Role: "system", Text: assignText, Ts: time.Now().Format("15:04:05"), PersonaName: "决策大脑", AvatarFile: "system.png"})
		if db.webOutputChan != nil {
			assignMsg := WebMsg{Type: "UserMessage", Data: map[string]interface{}{
				"persona_name": "决策大脑",
				"avatar_file":  "system.png",
				"agent_id":     "",
				"message":      assignText,
			}, ProjectName: db.projectName}
			if b, err := json.Marshal(assignMsg); err == nil {
				db.trySendWS(string(b))
			}
		}
	}
	// Set TeamMessage handler: broadcast to all other running agents' memories.
	agent.GetMemory().SetTeamMessageHandler(func(senderCtxId string, tmsg string) {
		db.broadcastTeamMessage(senderCtxId, senderPersonaName, tmsg)
	})
	// Set UserMessage handler: send to frontend only (not to other agents).
	agent.GetMemory().SetUserMessageHandler(func(senderCtxId string, umsg string) {
		db.AppendChatMessage(ChatMessage{Role: "system", Text: umsg, Ts: time.Now().Format("15:04:05"), PersonaName: senderPersonaName, AvatarFile: senderAvatarFile})
		if db.webOutputChan != nil {
			msg := WebMsg{
				Type: "UserMessage",
				Data: map[string]interface{}{
					"persona_name": senderPersonaName,
					"avatar_file":  senderAvatarFile,
					"agent_id":     agent.GetId(),
					"message":      umsg,
				},
				ProjectName: db.projectName,
			}
			if b, err := json.Marshal(msg); err == nil {
				db.trySendWS(string(b))
			}
		}
	})
	// Set BrainMessage handler: unicast message from digital human to DecisionBrain.
	agent.GetMemory().SetBrainMessageHandler(func(senderCtxId string, bmsg string) {
		chatText := "@决策大脑 " + bmsg
		db.AppendChatMessage(ChatMessage{Role: "system", Text: chatText, Ts: time.Now().Format("15:04:05"), PersonaName: senderPersonaName, AvatarFile: senderAvatarFile})
		if db.webOutputChan != nil {
			msg := WebMsg{
				Type: "UserMessage",
				Data: map[string]interface{}{
					"persona_name": senderPersonaName,
					"avatar_file":  senderAvatarFile,
					"agent_id":     agent.GetId(),
					"message":      chatText,
				},
				ProjectName: db.projectName,
			}
			if b, err := json.Marshal(msg); err == nil {
				db.trySendWS(string(b))
			}
		}
		brainContent := fmt.Sprintf("[BrainMessage from %s] %s\n\n(If you need to reply to %s, use Tool-SendMessageToDigitalHuman. Do NOT use <UserMessage> to reply — that is only for broadcasting to the user.)", senderPersonaName, bmsg, senderPersonaName)
		db.memory.AddMessage(llm.Message{
			Role:    llm.RoleUser,
			Content: brainContent,
		})
		db.signal()
	})
	ar := &AgentRuntime{agent: agent, agentToolName: name, Resp: &agents.StartResp{}, startedAt: time.Now(), done: false}
	db.runAgentList = append(db.runAgentList, ar)
	db.poolMu.Lock()
	db.digitalHumanBusy[agent.GetId()] = digitalHumanBusyInfo{AgentToolName: name, Profile: profile, Agent: agent}
	if dhID := strings.TrimSpace(profile.DigitalHumanID); dhID != "" {
		db.lastArgs[dhID] = args
	}
	db.updateDigitalHumanRosterMemoryLocked()
	db.poolMu.Unlock()
	// If there is a follow-up user message (e.g. from @-mention reactivation),
	// inject it into the agent's memory so the agent sees it as a user question.
	if strings.TrimSpace(followUpUserMsg) != "" {
		agent.GetMemory().AddMessage(&llm.MessageX{
			ContextId: task.GetTaskId(),
			Msg: llm.Message{
				Role:    llm.RoleUser,
				Content: "[Team Chat - to you] " + followUpUserMsg + "\n\n(You MUST reply to this message using <UserMessage>your reply</UserMessage> in your next response.)",
			},
		})
	}
	// DoneCb is called by the agent's StartTask loop when executeTask finishes.
	doneCb := func(r *agents.StartResp) {
		db.AgentRunDone(agent.GetId(), r)
	}
	ready := 1
	go func() {
		if isDeferred {
			ready = 0
			db.envBuildCond.L.Lock()
			for len(db.envInfo) == 0 {
				select {
				case <-db.done:
					db.envBuildCond.L.Unlock()
					misc.Debug("[%s] 等待环境搭建期间 db.done 关闭，释放数字人", agent.GetProfile().PersonaName)
					db.AgentRunDone(agent.GetId(), &agents.StartResp{Err: fmt.Errorf("cancelled: project stopped while waiting for env build")})
					return
				case <-db.ctx.Done():
					db.envBuildCond.L.Unlock()
					misc.Debug("[%s] 等待环境搭建期间 ctx 取消，释放数字人", agent.GetProfile().PersonaName)
					db.AgentRunDone(agent.GetId(), &agents.StartResp{Err: fmt.Errorf("cancelled: context done while waiting for env build")})
					return
				default:
				}
				if agent.GetMemory().HasPendingUserMessage() {
					misc.Debug("[%s] 等待环境搭建期间收到@消息，使用临时memory副本执行一轮对话", agent.GetProfile().PersonaName)
					pendingMsgs := agent.GetMemory().PopPendingUserMessages()
					// Fire-and-forget: run a one-shot LLM reply in a separate goroutine
					// using a temporary message list. The agent's real memory is untouched.
					go func(msgs []llm.Message) {
						tmpMessages := []llm.Message{
							{Role: llm.RoleSystem, Content: "你是一个漏洞验证专家（数字人），目前正在等待测试环境搭建完成。你暂时无法执行任何验证或测试操作。请用中文简短回复用户的消息，告知你正在等待环境就绪，环境搭建完成后会立即开始工作。不要编造任何信息。"},
						}
						tmpMessages = append(tmpMessages, msgs...)
						model := misc.GetConfigValueDefault("verifier", "MODEL", misc.GetConfigValueRequired("main_setting", "MODEL"))
						cli := llm.GetResponsesClient("verifier", "main_setting")
						resp, err := cli.Chat(db.ctx, model, tmpMessages, nil)
						if err != nil {
							misc.Debug("[%s] 临时对话LLM调用失败: %s", agent.GetProfile().PersonaName, err.Error())
							return
						}
						replyText := strings.TrimSpace(resp.Content)
						if replyText == "" {
							return
						}
						// Send the reply to the frontend as a UserMessage from this digital human.
						db.AppendChatMessage(ChatMessage{Role: "system", Text: replyText, Ts: time.Now().Format("15:04:05"), PersonaName: senderPersonaName, AvatarFile: senderAvatarFile})
						if db.webOutputChan != nil {
							msg := WebMsg{
								Type: "UserMessage",
								Data: map[string]interface{}{
									"persona_name": senderPersonaName,
									"avatar_file":  senderAvatarFile,
									"agent_id":     agent.GetId(),
									"message":      replyText,
								},
								ProjectName: db.projectName,
							}
							if b, err := json.Marshal(msg); err == nil {
								db.trySendWS(string(b))
							}
						}
					}(pendingMsgs)
					// Continue waiting for env build — do NOT break out of the loop.
				}
				db.envBuildCond.Wait()
			}
			db.envBuildCond.L.Unlock()
			select {
			case <-db.done:
				misc.Debug("[%s] 环境搭建等待结束后 db.done 已关闭，释放数字人", agent.GetProfile().PersonaName)
				db.AgentRunDone(agent.GetId(), &agents.StartResp{Err: fmt.Errorf("cancelled: project stopped after env wait")})
				return
			default:
			}
			if agent != nil {
				agent.SetEnvInfo(db.envInfo)
			}
			ready = 1
		}
		// For deferred agents (Verifier/Report), emit the chat assignment message now.
		if isDeferred {
			db.AppendChatMessage(ChatMessage{Role: "system", Text: assignText, Ts: time.Now().Format("15:04:05"), PersonaName: "决策大脑", AvatarFile: "system.png"})
			if db.webOutputChan != nil {
				assignMsg := WebMsg{Type: "UserMessage", Data: map[string]interface{}{
					"persona_name": "决策大脑",
					"avatar_file":  "system.png",
					"agent_id":     "",
					"message":      assignText,
				}, ProjectName: db.projectName}
				if b, err := json.Marshal(assignMsg); err == nil {
					db.trySendWS(string(b))
				}
			}
		}
		s := WebMsg{Type: "AgentStart", Data: map[string]interface{}{"AgentID": agent.GetId()}, ProjectName: db.projectName}
		js, _ := json.Marshal(s)
		db.trySendWS(string(js))
		// For analysis agents, inject the current exploitIdea summary so they see what's already been found.
		if strings.Contains(name, "Analyze") && len(db.exploitIdeaList) > 0 {
			db.injectExploitIdeaSummary(agent)
		}
		// Send task to the persistent agent via AssignTask (the agent's StartTask loop picks it up).
		agent.AssignTask(agents.TaskAssignment{ArgsJson: args, DoneCb: doneCb})
	}()
	time.Sleep(2 * time.Second)
	if ready == 0 {
		return "Since the test environment has not been successfully set up yet, the current verification Agent has been added to the execution queue. It will automatically run once the environment setup is completed. Agent ID: " + agent.GetId()
	}
	return "Agent ran successfully, please wait for the execution results. Agent ID: " + agent.GetId()
}

// broadcastTeamMessage inserts a team message from one digital human into all other running agents' memories.
func (db *DecisionBrain) broadcastTeamMessage(senderCtxId string, senderName string, tmsg string) {
	content := fmt.Sprintf("[TeamMessage from %s] %s", senderName, tmsg)
	misc.Success("TeamMessage", content, db.SubmitEventHandler)
	// Persist and push to frontend chat panel.
	var senderAvatar string
	db.poolMu.Lock()
	for _, info := range db.digitalHumanBusy {
		if info.Profile.PersonaName == senderName {
			senderAvatar = info.Profile.AvatarFile
			break
		}
	}
	db.poolMu.Unlock()
	db.AppendChatMessage(ChatMessage{Role: "system", Text: tmsg, Ts: time.Now().Format("15:04:05"), PersonaName: senderName, AvatarFile: senderAvatar})
	if db.webOutputChan != nil {
		msg := WebMsg{
			Type: "TeamMessage",
			Data: map[string]interface{}{
				"persona_name": senderName,
				"avatar_file":  senderAvatar,
				"message":      tmsg,
			},
			ProjectName: db.projectName,
		}
		if b, err := json.Marshal(msg); err == nil {
			db.trySendWS(string(b))
		}
	}
	db.mu.Lock()
	runningAgents := make([]agents.Agent, 0, len(db.runAgentList))
	for _, ar := range db.runAgentList {
		if ar != nil && ar.agent != nil && !ar.done {
			runningAgents = append(runningAgents, ar.agent)
		}
	}
	db.mu.Unlock()
	for _, a := range runningAgents {
		taskId := a.GetTask().GetTaskId()
		if taskId == senderCtxId {
			continue
		}
		a.GetMemory().AddKeyMessage(&llm.EnvMessageX{
			Key:       "TeamMessage",
			Content:   content,
			AppendEnv: true,
			ContextId: taskId,
			NotShared: true,
		})
	}
}

// TeamChat handles a team chat message from the user.
// - @PersonaName message → insert user message into that digital human's memory
// - @all message → insert user message into ALL running digital humans' memories
// - plain message (no @) → insert user message into the DecisionBrain's memory
// Returns a summary of what happened.
func (db *DecisionBrain) TeamChat(raw string, sender string) string {
	raw = strings.TrimSpace(raw)
	misc.Debug("user@msg 收到原始消息: sender=%s, raw=%q", sender, raw)
	if raw == "" {
		misc.Debug("user@msg 消息为空，忽略")
		return "empty message"
	}

	// Parse @target
	if strings.HasPrefix(raw, "@") {
		spaceIdx := strings.IndexByte(raw, ' ')
		if spaceIdx < 0 {
			misc.Debug("user@msg @消息没有空格分隔，无法解析目标和内容")
			return "message body is empty"
		}
		target := raw[1:spaceIdx]
		body := strings.TrimSpace(raw[spaceIdx+1:])
		misc.Debug("user@msg 解析结果: target=%q, body=%q", target, body)
		if body == "" {
			misc.Debug("user@msg 消息体为空，忽略")
			return "message body is empty"
		}

		if target == "all" || target == "全体" {
			misc.Debug("user@msg 广播模式: target=%s", target)
			// Collect running agent references under lock, then inject outside lock
			// because AddMessage(user) may block while the agent is in an LLM request.
			type agentRef struct {
				mem  llm.Memory
				tid  string
				name string
				aid  string
			}
			db.mu.Lock()
			misc.Debug("user@msg 当前 runAgentList 长度: %d", len(db.runAgentList))
			refs := make([]agentRef, 0, len(db.runAgentList))
			for _, ar := range db.runAgentList {
				if ar != nil && ar.agent != nil && !ar.done {
					refs = append(refs, agentRef{
						mem:  ar.agent.GetMemory(),
						tid:  ar.agent.GetTask().GetTaskId(),
						name: ar.agent.GetProfile().PersonaName,
						aid:  ar.agent.GetId(),
					})
				} else if ar != nil {
					misc.Debug("user@msg 广播跳过: agent=%v, done=%v", ar.agent != nil, ar.done)
				}
			}
			db.mu.Unlock()
			for _, ref := range refs {
				misc.Debug("user@msg 广播注入消息到运行中的数字人: name=%s, agentId=%s, taskId=%s", ref.name, ref.aid, ref.tid)
				ref.mem.AddMessage(&llm.MessageX{
					ContextId: ref.tid,
					Msg: llm.Message{
						Role:    llm.RoleUser,
						Content: fmt.Sprintf("[Team Chat from %s - to all] %s\n\n(You MUST reply to this message using <UserMessage>your reply</UserMessage> in your next response.)", sender, body),
					},
				})
			}
			misc.Debug("user@msg 广播完成，共注入 %d 个数字人，同时发送给决策大脑", len(refs))
			// Wake up Verifier/Report agents that may be waiting for env build.
			db.envBuildCond.L.Lock()
			db.envBuildCond.Broadcast()
			db.envBuildCond.L.Unlock()
			// Also send to the DecisionBrain so it can act on the message.
			db.memory.AddMessage(llm.Message{
				Role:    llm.RoleUser,
				Content: fmt.Sprintf("[Team Chat from %s - to all] %s\n\n(You MUST reply to this message using <UserMessage>your reply</UserMessage> in your next response.)", sender, body),
			})
			db.signal()
			return fmt.Sprintf("message sent to all %d running digital humans and DecisionBrain", len(refs))
		}

		// Find specific digital human by PersonaName — first check running agents.
		// Collect reference under lock, inject outside lock because AddMessage(user)
		// may block while the agent is in an LLM request.
		misc.Debug("user@msg 定向模式: 查找运行中的数字人 target=%q", target)
		var foundMem llm.Memory
		var foundTaskId, foundAgentId string
		db.mu.Lock()
		misc.Debug("user@msg 当前 runAgentList 长度: %d", len(db.runAgentList))
		for _, ar := range db.runAgentList {
			if ar != nil && ar.agent != nil && !ar.done {
				pName := ar.agent.GetProfile().PersonaName
				misc.Debug("user@msg 遍历运行中数字人: personaName=%q, agentId=%s, done=%v, 匹配=%v", pName, ar.agent.GetId(), ar.done, pName == target)
				if pName == target {
					foundMem = ar.agent.GetMemory()
					foundTaskId = ar.agent.GetTask().GetTaskId()
					foundAgentId = ar.agent.GetId()
					break
				}
			} else if ar != nil {
				agentName := ""
				if ar.agent != nil {
					agentName = ar.agent.GetProfile().PersonaName
				}
				misc.Debug("user@msg 跳过已完成/空agent: personaName=%q, agent!=nil=%v, done=%v", agentName, ar.agent != nil, ar.done)
			}
		}
		db.mu.Unlock()
		if foundMem != nil {
			misc.Debug("user@msg 匹配成功！注入消息到运行中的数字人: target=%s, agentId=%s, taskId=%s, memoryType=%s", target, foundAgentId, foundTaskId, foundMem.GetType())
			foundMem.AddMessage(&llm.MessageX{
				ContextId: foundTaskId,
				Msg: llm.Message{
					Role:    llm.RoleUser,
					Content: fmt.Sprintf("[Team Chat from %s - to you] %s\n\n(You MUST reply to this message using <UserMessage>your reply</UserMessage> in your next response.)", sender, body),
				},
			})
			misc.Debug("user@msg 定向消息注入完成（运行中数字人），target=%s", target)
			// Wake up Verifier/Report agents that may be waiting for env build.
			db.envBuildCond.L.Lock()
			db.envBuildCond.Broadcast()
			db.envBuildCond.L.Unlock()
			return ""
		}

		// Digital human not running — find idle one in pool and reactivate.
		misc.Debug("user@msg 运行中未找到 target=%q，开始在空闲池中查找", target)
		db.poolMu.Lock()
		var reactivateToolName string
		var reactivateDHID string
		for toolName, pool := range db.digitalHumanPool {
			for _, entry := range pool {
				p := entry.Profile
				misc.Debug("user@msg 空闲池遍历: toolName=%s, personaName=%q, dhID=%s, 匹配=%v", toolName, p.PersonaName, p.DigitalHumanID, p.PersonaName == target)
				if p.PersonaName == target {
					reactivateToolName = toolName
					reactivateDHID = strings.TrimSpace(p.DigitalHumanID)
					break
				}
			}
			if reactivateToolName != "" {
				break
			}
		}
		// Look up the original task args for this digital human.
		originalArgs := db.lastArgs[reactivateDHID]
		db.poolMu.Unlock()
		if reactivateToolName != "" {
			misc.Debug("user@msg 空闲池找到! target=%s, toolName=%s, dhID=%s, hasOriginalArgs=%v", target, reactivateToolName, reactivateDHID, originalArgs != "")
			if originalArgs == "" {
				misc.Debug("user@msg 无历史任务参数，以用户消息作为新任务启动")
				// No previous task recorded — fall back to using the user message as a new task.
				go db.startAgentWithFollowUp(reactivateToolName, fmt.Sprintf(`{"task_content":"[Team Chat from %s - to you] %s"}`, sender, body), "", reactivateDHID)
			} else {
				misc.Debug("user@msg 使用历史任务参数重新激活，并注入用户消息作为 followUp")
				// Reactivate with original task + inject user message as follow-up.
				go db.startAgentWithFollowUp(reactivateToolName, originalArgs, body, reactivateDHID)
			}
			return ""
		}
		misc.Debug("user@msg 在运行列表和空闲池中均未找到 target=%q", target)
		// 打印当前所有空闲池内容供调试
		db.poolMu.Lock()
		for tn, pool := range db.digitalHumanPool {
			for _, entry := range pool {
				misc.Debug("user@msg 空闲池剩余: toolName=%s, personaName=%q, dhID=%s", tn, entry.Profile.PersonaName, entry.Profile.DigitalHumanID)
			}
		}
		for aid, b := range db.digitalHumanBusy {
			misc.Debug("user@msg 忙碌列表: agentId=%s, toolName=%s, personaName=%q, dhID=%s", aid, b.AgentToolName, b.Profile.PersonaName, b.Profile.DigitalHumanID)
		}
		db.poolMu.Unlock()
		return fmt.Sprintf("digital human '%s' not found", target)
	}

	// No @ prefix → insert into DecisionBrain memory.
	db.memory.AddMessage(llm.Message{
		Role:    llm.RoleUser,
		Content: fmt.Sprintf("[Team Chat from %s] %s\n\n(You MUST reply to this message using <UserMessage>your reply</UserMessage> in your next response.)", sender, raw),
	})
	db.signal()
	misc.Debug("收到用户消息：%s", fmt.Sprintf("[Team Chat from %s] %s\n\n(You MUST reply to this message using <UserMessage>your reply</UserMessage> in your next response.)", sender, raw))
	return ""
}

// AppendChatMessage adds a message to the chat log and persists to disk.
func (db *DecisionBrain) AppendChatMessage(msg ChatMessage) {
	db.chatMu.Lock()
	db.chatMessages = append(db.chatMessages, msg)
	if len(db.chatMessages) > maxDecisionChatRecord {
		db.chatMessages = append([]ChatMessage(nil), db.chatMessages[len(db.chatMessages)-maxDecisionChatRecord:]...)
	}
	now := time.Now()
	if db.lastChatPersistAt.IsZero() || now.Sub(db.lastChatPersistAt) >= chatPersistDebounce {
		db.saveChatMessagesLocked()
		db.lastChatPersistAt = now
		if db.chatPersistTimer != nil {
			db.chatPersistTimer.Stop()
			db.chatPersistTimer = nil
		}
	} else if db.chatPersistTimer == nil {
		wait := chatPersistDebounce - now.Sub(db.lastChatPersistAt)
		if wait < 100*time.Millisecond {
			wait = 100 * time.Millisecond
		}
		db.chatPersistTimer = time.AfterFunc(wait, func() {
			db.chatMu.Lock()
			defer db.chatMu.Unlock()
			db.saveChatMessagesLocked()
			db.lastChatPersistAt = time.Now()
			db.chatPersistTimer = nil
		})
	}
	db.chatMu.Unlock()
}

// BroadcastChatMessage appends the chat message AND pushes it to the frontend via WebSocket.
func (db *DecisionBrain) BroadcastChatMessage(msg ChatMessage) {
	db.AppendChatMessage(msg)
	if db.webOutputChan != nil {
		wsMsg := WebMsg{Type: "UserMessage", Data: map[string]interface{}{
			"persona_name": msg.PersonaName,
			"avatar_file":  msg.AvatarFile,
			"message":      msg.Text,
		}, ProjectName: db.projectName}
		if b, err := json.Marshal(wsMsg); err == nil {
			db.trySendWS(string(b))
		}
	}
}

// pushTokenUsage sends the current cumulative token usage to the frontend via WebSocket.
func (db *DecisionBrain) pushTokenUsage() {
	if db.webOutputChan == nil {
		return
	}
	pu := llm.GetProjectTokenUsage(db.projectName)
	usage := pu.Snapshot()
	msg := WebMsg{Type: "TokenUsage", Data: map[string]interface{}{
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_tokens":      usage.TotalTokens,
		"agents":            pu.AgentSnapshots(),
	}, ProjectName: db.projectName}
	if b, err := json.Marshal(msg); err == nil {
		db.trySendWS(string(b))
	}
}

// pushContextBreakdown sends the current brain context token breakdown to the frontend via WebSocket.
func (db *DecisionBrain) pushContextBreakdown() {
	if db.webOutputChan == nil {
		return
	}
	bd := db.memory.GetContextBreakdown()
	msg := WebMsg{Type: "BrainContextBreakdown", Data: bd, ProjectName: db.projectName}
	if b, err := json.Marshal(msg); err == nil {
		db.trySendWS(string(b))
	}
}

// GetContextBreakdown returns the current brain context token breakdown.
func (db *DecisionBrain) GetContextBreakdown() ContextBreakdown {
	return db.memory.GetContextBreakdown()
}

// GetTokenUsage returns the current cumulative token usage for this project.
func (db *DecisionBrain) GetTokenUsage() map[string]interface{} {
	pu := llm.GetProjectTokenUsage(db.projectName)
	usage := pu.Snapshot()
	return map[string]interface{}{
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_tokens":      usage.TotalTokens,
		"agents":            pu.AgentSnapshots(),
	}
}

func (db *DecisionBrain) GetTokenUsageSnapshot() taskManager.TokenUsageSnapshot {
	pu := llm.GetProjectTokenUsage(db.projectName)
	usage := pu.Snapshot()
	agentSnapshots := pu.AgentSnapshots()
	agents := make([]taskManager.AgentTokenUsageSnapshot, 0, len(agentSnapshots))
	for _, a := range agentSnapshots {
		agents = append(agents, taskManager.AgentTokenUsageSnapshot{
			Label:            a.Label,
			PromptTokens:     a.PromptTokens,
			CompletionTokens: a.CompletionTokens,
			TotalTokens:      a.TotalTokens,
		})
	}
	return taskManager.TokenUsageSnapshot{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		Agents:           agents,
	}
}

func (db *DecisionBrain) RestoreTokenUsageSnapshot(snapshot taskManager.TokenUsageSnapshot) {
	agents := make([]llm.AgentUsageSnapshot, 0, len(snapshot.Agents))
	for _, a := range snapshot.Agents {
		agents = append(agents, llm.AgentUsageSnapshot{
			Label:            a.Label,
			PromptTokens:     a.PromptTokens,
			CompletionTokens: a.CompletionTokens,
			TotalTokens:      a.TotalTokens,
		})
	}
	llm.RestoreProjectTokenUsage(db.projectName, snapshot.PromptTokens, snapshot.CompletionTokens, snapshot.TotalTokens, agents)
}

func (db *DecisionBrain) RestoreExploitState(ideas []*taskManager.ExploitIdea, chains []*taskManager.ExploitChain) {
	db.exploitIdeaList = make([]*taskManager.ExploitIdea, 0, len(ideas))
	for _, one := range ideas {
		if one != nil {
			db.exploitIdeaList = append(db.exploitIdeaList, one)
		}
	}
	db.exploitChainList = make([]*taskManager.ExploitChain, 0, len(chains))
	for _, one := range chains {
		if one != nil {
			db.exploitChainList = append(db.exploitChainList, one)
		}
	}
	db.flushExploitIdeaList()
	db.flushExploitChainList()
}

// GetChatMessages returns all persisted chat messages.
func (db *DecisionBrain) GetChatMessages() []ChatMessage {
	db.chatMu.Lock()
	defer db.chatMu.Unlock()
	out := make([]ChatMessage, len(db.chatMessages))
	copy(out, db.chatMessages)
	return out
}

func (db *DecisionBrain) saveChatMessagesLocked() {
	data, err := json.Marshal(db.chatMessages)
	if err != nil {
		return
	}
	_ = os.WriteFile(db.chatFilePath, data, 0644)
}

// 运行时证据审核
func (db *DecisionBrain) auditEvidence(evidence string, condition string, harm string) error {
	up := fmt.Sprintf("ExploitIdea: <condition>%s</condition>\n<harm>%s</harm>\n<evidence>%s</evidence>", condition, harm, evidence)
	if condition == "" && harm == "" {
		up = fmt.Sprintf("ExploitChain: <evidence>%s</evidence>", evidence)
	}
	cli := llm.GetResponsesClient("decision", "main_setting")
	var ms []llm.Message
	ms = append(ms, llm.Message{
		Role:    llm.RoleSystem,
		Content: auditPrompt,
	})
	ms = append(ms, llm.Message{
		Role:    llm.RoleUser,
		Content: up,
	})
	r, e := llm.RequestLLM(cli, context.Background(), db.model, ms, nil, db.projectName)
	if e != nil {
		return e
	}
	x := strings.TrimSpace(r.Content)
	if strings.ToLower(x) == "true" {
		return nil
	}
	return fmt.Errorf("%s", x)
}

// agentTypeToolInfo returns a human-readable description of the tools and
// constraints for a given agent tool name (e.g. "Agent-Analyze-AnalyzeCommonAgent").
func agentTypeToolInfo(agentToolName string) (typeName string, toolDesc string) {
	switch {
	case strings.Contains(agentToolName, "Analyze"):
		return "代码分析数字人（Analyze）", `可用工具：DetectLanguageTool（语言检测）、ListSourceCodeTreeTool（目录树）、SearchFileContentsByRegexTool（正则搜索）、ReadLinesFromFileTool（读取文件）、TaskListTool（任务管理）、GuidanceTool（专家咨询）、IssueCandidateExploitIdeaTool（提交漏洞线索）、IssueTool（反馈问题）
限制：只能读取源码，不能执行任何命令，不能操作容器，不能访问运行中的目标环境。`
	case strings.Contains(agentToolName, "Verifier"):
		return "漏洞验证数字人（Verifier）", `可用工具：DetectLanguageTool、ListSourceCodeTreeTool、SearchFileContentsByRegexTool、ReadLinesFromFileTool、TaskListTool、GuidanceTool、RunCommandTool（在容器内执行命令）、RunPythonCodeTool（执行Python代码）、RunPHPCodeTool（执行PHP代码）、RunSQLTool（执行SQL）、SubmitExploitIdeaTool（提交验证结果）、SubmitExploitChainTool（提交利用链结果）、DockerLogsTool（查看容器日志）、DockerDirScanTool（扫描容器目录）、DockerFileReadTool（读取容器文件）、GetExploitIdeaByIdTool、GetExploitChainByIdTool、IssueTool
限制：可以在已搭建的目标环境中执行命令和代码，但不能搭建或修改环境本身，不能操作宿主机。`
	case strings.Contains(agentToolName, "Ops") && strings.Contains(agentToolName, "Scout"):
		return "环境侦察数字人（OpsEnvScout）", `可用工具：RunCommandTool、DetectLanguageTool、DockerRunTool、DockerLogsTool、DockerRemoveTool、DockerExecTool、ListSourceCodeTreeTool、SearchFileContentsByRegexTool、ReadLinesFromFileTool、TaskListTool、GuidanceTool、IssueTool
限制：用于在已有测试环境中收集信息（用户名、密码、数据库、URL等），不负责漏洞分析。`
	case strings.Contains(agentToolName, "Ops"):
		return "运维数字人（Ops）", `可用工具：RunCommandTool（执行命令）、DetectLanguageTool、DockerRunTool（创建容器）、DockerLogsTool（容器日志）、DockerRemoveTool（删除容器）、DockerExecTool（容器内执行）、EnvSaveTool（保存环境信息）、RunSQLTool、JavaEnvTool、PHPEnvTool、NodeEnvTool、PythonEnvTool、GolangEnvTool、MySQLEnvTool、RedisEnvTool、ListSourceCodeTreeTool、SearchFileContentsByRegexTool、ReadLinesFromFileTool、TaskListTool、GuidanceTool、IssueTool
限制：负责环境搭建和维护，可以操作Docker容器，但不负责漏洞分析或验证。`
	case strings.Contains(agentToolName, "Report"):
		return "报告数字人（Report）", `可用工具：ListSourceCodeTreeTool、SearchFileContentsByRegexTool、ReadLinesFromFileTool、IssueTool、GuidanceTool、ReportVulnTool（生成报告）、GetExploitIdeaByIdTool、GetExploitChainByIdTool
限制：只负责撰写漏洞报告，不能执行命令，不能操作环境，只能读取源码和已有的漏洞数据。`
	case strings.Contains(agentToolName, "General"):
		return "通用杂项数字人（General）", `可用工具：RunCommandTool、DetectLanguageTool、ListSourceCodeTreeTool、SearchFileContentsByRegexTool、ReadLinesFromFileTool、RunPythonCodeTool、RunPHPCodeTool、RunSQLTool、DockerLogsTool、DockerDirScanTool、DockerFileReadTool、TaskListTool、GuidanceTool、IssueTool，以及已注入的 GitHub MCP 工具
限制：用于处理杂项任务（例如仓库协作、脚本执行、定位问题等）；涉及高风险环境变更时应先明确目标并谨慎执行。`
	default:
		return "未知类型数字人", "工具信息不详。"
	}
}

// Agent任务中产生难题指导
// Guidance inherits the current brain memory as read-only context (no new
// messages are written to db.memory). The system prompt is replaced with
// guidancePromptTemplate so the LLM answers in a guidance-specific role while
// still having full awareness of the project state.
func (db *DecisionBrain) Guidance(source string, q string) string {
	cli := llm.GetResponsesClient("decision", "main_setting")
	// Snapshot the brain's current context (read-only, does not mutate db.memory).
	snapshot := db.memory.GetContext()

	// Build source info block for the prompt.
	sourceInfo := "- 提问者: " + source
	extraConstraints := ""

	// Try to find the running agent to get its tool name for richer context.
	db.mu.Lock()
	for _, a := range db.runAgentList {
		if a.agent != nil && a.agent.GetProfile().PersonaName != "" &&
			strings.Contains(source, a.agent.GetProfile().PersonaName) {
			typeName, toolDesc := agentTypeToolInfo(a.agentToolName)
			sourceInfo = fmt.Sprintf("- 提问者: %s\n- 角色类型: %s\n- %s\n%s", source, typeName, toolDesc, "提问者并没有状态面板的信息，如果他有需要可以将对应信息发给他")
			break
		}
	}
	db.mu.Unlock()

	prompt := fmt.Sprintf(guidancePromptTemplate, sourceInfo, extraConstraints)

	// Build a new message list: replace the system prompt with enriched guidance prompt,
	// keep all other messages for context, then append the guidance question.
	ms := make([]llm.Message, 0, len(snapshot)+2)
	ms = append(ms, llm.Message{
		Role:    llm.RoleSystem,
		Content: prompt,
	})
	// Copy conversation history (skip the original system prompt).
	for _, m := range snapshot {
		if m.Role == llm.RoleSystem {
			continue
		}
		ms = append(ms, m)
	}
	ms = append(ms, llm.Message{
		Role:    llm.RoleUser,
		Content: q,
	})
	r, e := llm.RequestLLM(cli, context.Background(), db.model, ms, nil, db.projectName)
	if e != nil {
		return "决策大脑暂时无法响应。"
	}
	content := r.Content
	s := WebMsg{Type: "Guidance", Data: map[string]interface{}{"source": source, "q": q, "a": content}, ProjectName: db.projectName}
	js, _ := json.Marshal(s)
	db.trySendWS(string(js))
	return content
}

func (db *DecisionBrain) AgentRunDone(agentId string, r *agents.StartResp) {
	s1 := WebMsg{Type: "AgentDone", Data: map[string]interface{}{"AgentID": agentId, "Resp": r.Summary}, ProjectName: db.projectName}
	js1, _ := json.Marshal(s1)
	db.trySendWS(string(js1))

	// ---- Step 1: Immediately mark agent as done so the brain sees it right away. ----
	var a *AgentRuntime
	for _, v := range db.runAgentList {
		if v.agent.GetId() == agentId {
			a = v
		}
	}
	if a == nil {
		db.releaseDigitalHuman(agentId)
		return
	}
	if a.agent != nil {
		a.agent.SetState("Done")
	}
	a.done = true
	a.Resp = r

	// Store last task and summary for this digital human so the roster can show it.
	truncatedSummary := r.Summary
	if runes := []rune(truncatedSummary); len(runes) > 500 {
		truncatedSummary = string(runes[:500]) + "..."
	}
	var runTask string
	for i, t := range a.agent.GetTask().GetTaskList() {
		runTask += fmt.Sprintf("task.%d: %s\n", i, t["TaskContent"])
	}
	db.poolMu.Lock()
	busy, hasBusy := db.digitalHumanBusy[agentId]
	if hasBusy {
		if dhID := strings.TrimSpace(busy.Profile.DigitalHumanID); dhID != "" {
			db.lastSummary[dhID] = truncatedSummary
			db.lastTask[dhID] = runTask
		}
	}
	db.poolMu.Unlock()

	// Release the slot — any @message will correctly go to the reactivation path.
	db.releaseDigitalHuman(agentId)

	// ---- Verifier cleanup: reset "正在验证" → "等待验证" if the agent finished
	// without submitting a result (i.e. the state was never updated by SubmitExploitIdeaHandler/SubmitExploitChainHandler). ----
	if hasBusy && strings.Contains(busy.AgentToolName, "Verifier") {
		if dhID := strings.TrimSpace(busy.Profile.DigitalHumanID); dhID != "" {
			if argsStr, ok := db.lastArgs[dhID]; ok {
				var argsMap map[string]interface{}
				if json.Unmarshal([]byte(argsStr), &argsMap) == nil {
					if eid, ok := argsMap["exploit_idea_id"]; ok && fmt.Sprint(eid) != "" {
						if ei, err := db.GetExploitIdeaById(fmt.Sprint(eid)); err == nil && ei.State == "正在验证" {
							ei.State = "等待验证"
							misc.Debug("AgentRunDone: Verifier %s 结束但 ExploitIdea %s 仍为正在验证，重置为等待验证", busy.Profile.PersonaName, fmt.Sprint(eid))
							db.flushExploitIdeaList()
						}
					}
					if cid, ok := argsMap["exploit_chain_id"]; ok && fmt.Sprint(cid) != "" {
						if ec, err := db.GetExploitChainById(fmt.Sprint(cid)); err == nil && ec.State == "正在验证" {
							ec.State = "等待验证"
							misc.Debug("AgentRunDone: Verifier %s 结束但 ExploitChain %s 仍为正在验证，重置为等待验证", busy.Profile.PersonaName, fmt.Sprint(cid))
							db.flushExploitChainList()
						}
					}
				}
			}
		}
	}

	// Update runtime info and wake the brain immediately.
	compact := make([]interface{}, 0, len(db.runAgentList))
	for _, v := range db.runAgentList {
		compact = append(compact, v.GetRunInfo())
	}
	js, _ := json.Marshal(compact)
	db.memory.UpdateAgentRuntimeInfo(string(js))
	db.signal()

	// ---- Step 2: Summarize persona memory asynchronously (slow LLM call). ----
	// This no longer blocks the state update, so the brain sees "Done" right away.
	if hasBusy {
		ctxMsgs := a.agent.GetMemory().GetContext(a.agent.GetTask().GetTaskId())
		go func(busyInfo digitalHumanBusyInfo, runTask string, summary string, ctxMsgs []llm.Message) {
			existing := db.getPersonaMemory(busyInfo.AgentToolName, busyInfo.Profile)
			updated := db.summarizePersonaMemory(busyInfo.AgentToolName, busyInfo.Profile, existing, runTask, summary, ctxMsgs)
			if strings.TrimSpace(updated) != "" {
				db.setPersonaMemory(busyInfo.AgentToolName, busyInfo.Profile, updated)
			}
		}(busy, runTask, r.Summary, ctxMsgs)
	}
}

func (db *DecisionBrain) SetEnvInfo(env map[string]interface{}) {
	s := WebMsg{Type: "EnvInfo", Data: map[string]interface{}{"env": env}, ProjectName: db.projectName}
	js1, _ := json.Marshal(s)
	db.trySendWS(string(js1))

	db.envBuildCond.L.Lock()
	db.envInfo = env
	db.envBuildCond.Broadcast()
	db.envBuildCond.L.Unlock()
	js, _ := json.Marshal(env)
	db.memory.SetEnvInfo(string(js))
	db.signal()
}

// 组合利用链
func (db *DecisionBrain) SynthesizeChain(chainIdList []string, idea string) error {
	ec := &taskManager.ExploitChain{Idea: idea, State: "未验证"}
	ec.ExploitIdea = make([]*taskManager.ExploitIdea, 0, len(chainIdList))
	for _, v := range chainIdList {
		e, err := db.GetExploitIdeaById(v)
		if err != nil {
			return err
		}
		ec.ExploitIdea = append(ec.ExploitIdea, e)
	}
	index := 0
	for {
		id := fmt.Sprintf("C.%d", index)
		exists := false
		for _, v := range db.exploitChainList {
			if v != nil && v.ExploitChainId == id {
				exists = true
				break
			}
		}
		if !exists {
			ec.ExploitChainId = id
			break
		}
		index++
	}
	db.exploitChainList = append(db.exploitChainList, ec)
	err := db.VerifyExploitChain(ec.ExploitChainId)
	if err != nil {
		return err
	}
	db.flushExploitChainList()
	return nil
}
