package DecisionBrain

import (
	"AIxVuln/agents"
	"fmt"
	"strings"
	"time"
)

type AgentRuntime struct {
	agent         agents.Agent
	agentToolName string
	Resp          *agents.StartResp
	startedAt     time.Time
	done          bool
}

func (r *AgentRuntime) GetRunInfo() map[string]interface{} {
	rs := make(map[string]interface{})
	rs["AgentID"] = r.agent.GetId()
	state := r.agent.GetState()
	rs["RunState"] = state
	if p := r.agent.GetProfile(); p.PersonaName != "" {
		rs["persona_name"] = p.PersonaName
	}
	rs["StartedAt"] = r.startedAt.Format("2006-01-02 15:04:05")

	stateLower := strings.ToLower(strings.TrimSpace(state))
	isDone := stateLower == "done" || stateLower == "completed" || stateLower == "success"

	if isDone {
		// For done agents: only a short one-line summary, no task content.
		summary := r.Resp.Summary
		if runes := []rune(summary); len(runes) > 120 {
			summary = string(runes[:120]) + "..."
		}
		rs["RUNSummary"] = summary
	} else {
		// For active agents: include task and a moderate summary.
		var taskContent string
		for i, v := range r.agent.GetTask().GetTaskList() {
			taskContent += fmt.Sprintf("task.%d: %s\n", i, v["TaskContent"])
		}
		rs["RUNTask"] = taskContent
		summary := r.Resp.Summary
		if runes := []rune(summary); len(runes) > 300 {
			summary = string(runes[:300]) + "..."
		}
		rs["RUNSummary"] = summary
	}
	return rs
}

// GetRunInfoFull returns the full run info for UI display (not truncated).
func (r *AgentRuntime) GetRunInfoFull() map[string]interface{} {
	rs := make(map[string]interface{})
	rs["AgentID"] = r.agent.GetId()
	rs["RunState"] = r.agent.GetState()
	if p := r.agent.GetProfile(); p.PersonaName != "" || p.Gender != "" {
		rs["Profile"] = map[string]interface{}{
			"digital_human_id": p.DigitalHumanID,
			"persona_name":     p.PersonaName,
			"gender":           p.Gender,
			"avatar_file":      p.AvatarFile,
			"personality":      p.Personality,
			"age":              p.Age,
		}
	}
	var taskContent string
	for i, v := range r.agent.GetTask().GetTaskList() {
		taskContent += fmt.Sprintf("task.%d: %s\n", i, v["TaskContent"])
	}
	rs["RUNTask"] = taskContent
	rs["StartedAt"] = r.startedAt.Format("2006-01-02 15:04:05")
	rs["RUNSummary"] = r.Resp.Summary
	// Expose agent context usage for UI progress bar.
	if mem := r.agent.GetMemory(); mem != nil {
		rs["context_size"] = mem.GetMsgSize(r.agent.GetId())
		rs["max_context"] = mem.GetMaxHistory()
	}
	return rs
}

type WebMsg struct {
	Type        string      `json:"type"`
	Data        interface{} `json:"data"`
	ProjectName string      `json:"projectName"`
}
