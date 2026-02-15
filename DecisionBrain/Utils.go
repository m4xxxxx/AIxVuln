package DecisionBrain

import (
	"AIxVuln/misc"
	"AIxVuln/taskManager"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (db *DecisionBrain) GetEnvInfo() map[string]interface{} {
	return db.envInfo
}

func (db *DecisionBrain) GetContainerList() []*taskManager.ContainerInfo {
	return db.containerList
}

func (db *DecisionBrain) GetReportList() []map[string]string {
	return db.reportList
}

func (db *DecisionBrain) GetExploitIdeaList() []*taskManager.ExploitIdea {
	return db.exploitIdeaList
}

func (db *DecisionBrain) GetExploitChainList() []*taskManager.ExploitChain {
	return db.exploitChainList
}

func (db *DecisionBrain) GetEvent(count int) []string {
	db.feedMu.Lock()
	defer db.feedMu.Unlock()
	l := len(db.eventList)
	if count <= 0 || l <= count {
		out := make([]string, l)
		copy(out, db.eventList)
		return out
	}
	out := make([]string, count)
	copy(out, db.eventList[l-count:])
	return out
}

func (db *DecisionBrain) AppendBrainFeed(kind string, data map[string]interface{}) {
	db.feedMu.Lock()
	defer db.feedMu.Unlock()
	if db.brainFeed == nil {
		db.brainFeed = make([]map[string]interface{}, 0)
	}
	if data != nil {
		if _, ok := data["ts"]; !ok {
			data["ts"] = time.Now().Format("2006-01-02 15:04")
		}
	}
	item := map[string]interface{}{
		"kind": kind,
		"data": data,
	}
	db.brainFeed = append(db.brainFeed, item)
	if len(db.brainFeed) > 200 {
		db.brainFeed = db.brainFeed[len(db.brainFeed)-200:]
	}
}

// EmitBrainMessage emits a message as if it came from the DecisionBrain:
// appends to brain feed, sends via WebSocket, and broadcasts as a chat message.
func (db *DecisionBrain) EmitBrainMessage(content string) {
	data := map[string]interface{}{
		"role":    "assistant",
		"content": content,
	}
	db.AppendBrainFeed("BrainMessage", data)
	db.AppendChatMessage(ChatMessage{
		Role: "system", Text: content,
		Ts: time.Now().Format("15:04:05"), PersonaName: "决策大脑", AvatarFile: "system.png",
	})
	if db.webOutputChan != nil {
		chatMsg := WebMsg{Type: "UserMessage", Data: map[string]interface{}{
			"persona_name": "决策大脑",
			"avatar_file":  "system.png",
			"agent_id":     "",
			"message":      content,
		}, ProjectName: db.projectName}
		if b, err := json.Marshal(chatMsg); err == nil {
			db.trySendWS(string(b))
		}
	}
}

func (db *DecisionBrain) GetBrainFeed(count int) []map[string]interface{} {
	db.feedMu.Lock()
	defer db.feedMu.Unlock()
	l := len(db.brainFeed)
	if count <= 0 || l <= count {
		out := make([]map[string]interface{}, l)
		copy(out, db.brainFeed)
		return out
	}
	out := make([]map[string]interface{}, count)
	copy(out, db.brainFeed[l-count:])
	return out
}

func (db *DecisionBrain) AppendAgentFeed(agentID string, kind string, data map[string]interface{}) {
	if agentID == "" {
		return
	}
	db.feedMu.Lock()
	defer db.feedMu.Unlock()
	if db.agentFeed == nil {
		db.agentFeed = make(map[string][]map[string]interface{})
	}
	if data != nil {
		if _, ok := data["ts"]; !ok {
			data["ts"] = time.Now().Format("2006-01-02 15:04")
		}
	}
	list := db.agentFeed[agentID]
	item := map[string]interface{}{
		"kind": kind,
		"data": data,
	}
	list = append(list, item)
	if len(list) > 200 {
		list = list[len(list)-200:]
	}
	db.agentFeed[agentID] = list
}

func (db *DecisionBrain) GetAgentFeed(agentID string, count int) []map[string]interface{} {
	if agentID == "" {
		return nil
	}
	db.feedMu.Lock()
	defer db.feedMu.Unlock()
	if db.agentFeed == nil {
		return nil
	}
	list := db.agentFeed[agentID]
	l := len(list)
	if count <= 0 || l <= count {
		out := make([]map[string]interface{}, l)
		copy(out, list)
		return out
	}
	out := make([]map[string]interface{}, count)
	copy(out, list[l-count:])
	return out
}

func (db *DecisionBrain) SubmitAgentFeedHandler(agentID string, kind string, data map[string]interface{}) {
	if agentID == "" || kind == "" {
		return
	}
	if data == nil {
		data = map[string]interface{}{}
	}
	if _, ok := data["ts"]; !ok {
		data["ts"] = time.Now().Format("2006-01-02 15:04")
	}
	// cache
	db.AppendAgentFeed(agentID, kind, data)

	// Push token usage update after each agent LLM response.
	if kind == "AgentMessage" {
		db.pushTokenUsage()
	}
}

func (db *DecisionBrain) GetAgentRuntimeList() []map[string]interface{} {
	res := make([]map[string]interface{}, 0, len(db.runAgentList))
	for _, v := range db.runAgentList {
		if v == nil {
			continue
		}
		res = append(res, v.GetRunInfo())
	}
	return res
}

func (db *DecisionBrain) GetExploitIdeaById(id string) (*taskManager.ExploitIdea, error) {
	for _, v := range db.exploitIdeaList {
		if v.ExploitIdeaId == id {
			return v, nil
		}
	}
	return nil, errors.New("ExploitIdeaId not found: " + id)
}

func (db *DecisionBrain) GetExploitChainById(id string) (*taskManager.ExploitChain, error) {
	for _, v := range db.exploitChainList {
		if v.ExploitChainId == id {
			return v, nil
		}
	}
	return nil, errors.New("ExploitChainId not found: " + id)
}

func (db *DecisionBrain) WaitEnvBuildDone() {
	db.envBuildCond.L.Lock()
	defer db.envBuildCond.L.Unlock()
	for len(db.envInfo) == 0 {
		select {
		case <-db.done:
			return
		case <-db.ctx.Done():
			return
		default:
		}
		db.envBuildCond.Wait()
	}
}

func (db *DecisionBrain) VerifyExploitChain(cid string) error {
	ec, err := db.GetExploitChainById(cid)
	if err != nil {
		return err
	}
	args := map[string]string{"exploit_chain_id": cid}
	argsJson, _ := json.Marshal(args)
	startMsg := db.startAgent("Agent-Verifier-VerifierCommonAgent", string(argsJson))
	misc.Debug("VerifyExploitChain(%s): startAgent returned: %s", cid, startMsg)
	if strings.Contains(strings.ToLower(startMsg), "not found") {
		return fmt.Errorf("%s", startMsg)
	}
	if strings.Contains(startMsg, "Agent ran successfully") {
		ec.State = "正在验证"
	} else {
		ec.State = "等待验证"
	}

	wsMsg := WebMsg{Type: "ExploitChainUpdate", Data: ec, ProjectName: db.projectName}
	if b, err := json.Marshal(wsMsg); err == nil {
		db.trySendWS(string(b))
	}
	return nil
}

func (db *DecisionBrain) VerifyExploitIdea(eid string) error {
	e, err := db.GetExploitIdeaById(eid)
	if err != nil {
		return err
	}
	args := map[string]string{"exploit_idea_id": eid}
	argsJson, _ := json.Marshal(args)
	startMsg := db.startAgent("Agent-Verifier-VerifierCommonAgent", string(argsJson))
	misc.Debug("VerifyExploitIdea(%s): startAgent returned: %s", eid, startMsg)
	if strings.Contains(strings.ToLower(startMsg), "not found") {
		return fmt.Errorf("%s", startMsg)
	}
	if strings.Contains(startMsg, "Agent ran successfully") {
		e.State = "正在验证"
	} else {
		e.State = "等待验证"
	}

	wsMsg := WebMsg{Type: "ExploitIdeaUpdate", Data: e, ProjectName: db.projectName}
	if b, err := json.Marshal(wsMsg); err == nil {
		db.trySendWS(string(b))
	}
	return nil
}

func (db *DecisionBrain) AgentStateUpdate(state string) {
	compact := make([]interface{}, 0, len(db.runAgentList))
	full := make([]interface{}, 0, len(db.runAgentList))
	for _, v := range db.runAgentList {
		if v == nil {
			continue
		}
		compact = append(compact, v.GetRunInfo())
		full = append(full, v.GetRunInfoFull())
	}
	js, _ := json.Marshal(compact)
	db.memory.UpdateAgentRuntimeInfo(string(js))

	// Real-time panel update for UI (full info)
	msg := WebMsg{Type: "AgentRuntimeUpdate", Data: full, ProjectName: db.projectName}
	if b, err := json.Marshal(msg); err == nil {
		db.trySendWS(string(b))
	}
}
