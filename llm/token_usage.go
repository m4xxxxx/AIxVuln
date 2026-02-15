package llm

import (
	"sort"
	"sync"
	"sync/atomic"
)

// ProjectTokenUsage tracks cumulative token usage for a single project.
type ProjectTokenUsage struct {
	PromptTokens     atomic.Int64
	CompletionTokens atomic.Int64
	TotalTokens      atomic.Int64

	agentMu    sync.RWMutex
	agentUsage map[string]*AgentTokenUsage // key = agentLabel (e.g. persona name or "决策大脑")
}

// AgentTokenUsage tracks cumulative token usage for a single agent/digital human.
type AgentTokenUsage struct {
	PromptTokens     atomic.Int64
	CompletionTokens atomic.Int64
	TotalTokens      atomic.Int64
}

// AgentUsageSnapshot is a JSON-friendly snapshot of one agent's token usage.
type AgentUsageSnapshot struct {
	Label            string `json:"label"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

// Add accumulates usage from a single API call.
func (u *ProjectTokenUsage) Add(usage Usage) {
	u.PromptTokens.Add(usage.PromptTokens)
	u.CompletionTokens.Add(usage.CompletionTokens)
	u.TotalTokens.Add(usage.TotalTokens)
}

// AddAgent accumulates usage for a specific agent label within this project.
func (u *ProjectTokenUsage) AddAgent(agentLabel string, usage Usage) {
	if agentLabel == "" {
		return
	}
	u.agentMu.RLock()
	au, ok := u.agentUsage[agentLabel]
	u.agentMu.RUnlock()
	if !ok {
		u.agentMu.Lock()
		if au, ok = u.agentUsage[agentLabel]; !ok {
			au = &AgentTokenUsage{}
			if u.agentUsage == nil {
				u.agentUsage = make(map[string]*AgentTokenUsage)
			}
			u.agentUsage[agentLabel] = au
		}
		u.agentMu.Unlock()
	}
	au.PromptTokens.Add(usage.PromptTokens)
	au.CompletionTokens.Add(usage.CompletionTokens)
	au.TotalTokens.Add(usage.TotalTokens)
}

// Snapshot returns the current cumulative usage.
func (u *ProjectTokenUsage) Snapshot() Usage {
	return Usage{
		PromptTokens:     u.PromptTokens.Load(),
		CompletionTokens: u.CompletionTokens.Load(),
		TotalTokens:      u.TotalTokens.Load(),
	}
}

// AgentSnapshots returns per-agent usage sorted by total tokens descending.
func (u *ProjectTokenUsage) AgentSnapshots() []AgentUsageSnapshot {
	u.agentMu.RLock()
	defer u.agentMu.RUnlock()
	out := make([]AgentUsageSnapshot, 0, len(u.agentUsage))
	for label, au := range u.agentUsage {
		out = append(out, AgentUsageSnapshot{
			Label:            label,
			PromptTokens:     au.PromptTokens.Load(),
			CompletionTokens: au.CompletionTokens.Load(),
			TotalTokens:      au.TotalTokens.Load(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TotalTokens > out[j].TotalTokens })
	return out
}

var (
	projectUsageMu sync.RWMutex
	projectUsage   = make(map[string]*ProjectTokenUsage)
)

// GetProjectTokenUsage returns the token usage tracker for a project (creates if needed).
func GetProjectTokenUsage(projectName string) *ProjectTokenUsage {
	projectUsageMu.RLock()
	u, ok := projectUsage[projectName]
	projectUsageMu.RUnlock()
	if ok {
		return u
	}
	projectUsageMu.Lock()
	defer projectUsageMu.Unlock()
	if u, ok = projectUsage[projectName]; ok {
		return u
	}
	u = &ProjectTokenUsage{agentUsage: make(map[string]*AgentTokenUsage)}
	projectUsage[projectName] = u
	return u
}

// AddProjectTokenUsage is a convenience function to accumulate usage for a project.
func AddProjectTokenUsage(projectName string, usage Usage) {
	if usage.TotalTokens <= 0 {
		return
	}
	GetProjectTokenUsage(projectName).Add(usage)
}

// AddProjectAgentTokenUsage accumulates usage for both the project total and a specific agent.
func AddProjectAgentTokenUsage(projectName string, agentLabel string, usage Usage) {
	if usage.TotalTokens <= 0 {
		return
	}
	pu := GetProjectTokenUsage(projectName)
	pu.Add(usage)
	pu.AddAgent(agentLabel, usage)
}
