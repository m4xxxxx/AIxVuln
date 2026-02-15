package DecisionBrain

import (
	"AIxVuln/agents"
	"AIxVuln/llm"
	"AIxVuln/misc"
	"AIxVuln/toolCalling"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func getAgentTools() []llm.ToolDef {
	var definitions []llm.ToolDef
	_, info := agents.GetAgentDescription()
	for _, handler := range info {
		definitions = append(definitions, llm.ToolDef{
			Name:        handler["Name"].(string),
			Description: handler["Description"].(string),
			Parameters:  handler["args"].(map[string]interface{}),
		})
	}
	return definitions
}

func (db *DecisionBrain) Start() {
	var ready bool
	for {
		select {
		case <-db.done:
			return
		default:
		}
		db.memory.SetNoAgentRunning(!db.hasRunningAgents())
		msgList := db.memory.GetContext()
		if msgList == nil {
			select {
			case <-db.done:
				return
			case <-time.After(1 * time.Second):
			}
			continue
		}
		memLastHash := db.memory.GetLastHash()
		if !ready && memLastHash == db.lastMsgHash && db.lastMsgHash != "" {
			select {
			case <-db.notify:
			case <-db.done:
				return
			case <-time.After(30 * time.Second):
			}
			continue
		}
		db.lastMsgHash = memLastHash
		assistantMessage, toolMessage, err := db.ToolCallRequest(msgList)
		if err != nil {
			misc.Error("decision", err.Error(), db.SubmitEventHandler)
		}
		// If the LLM returned an empty response with no tool calls, do NOT add it
		// to memory — it would just bloat the context and cause an infinite loop.
		if len(toolMessage) == 0 && (strings.TrimSpace(assistantMessage.Content) == "" ||
			strings.HasPrefix(assistantMessage.Content, "empty response")) {
			misc.Debug("决策大脑：跳过空响应，等待新事件")
			ready = false
			select {
			case <-db.notify:
			case <-db.done:
				return
			case <-time.After(30 * time.Second):
			}
			continue
		}
		db.memory.AddMessage(assistantMessage)
		if err := db.memory.CompressIfNeeded(llm.GetResponsesClient("decision", "main_setting"), db.model); err != nil {
			misc.Error("decision", err.Error(), db.SubmitEventHandler)
		}

		// Check for special tool markers before adding tool messages to memory.
		hasWait := false
		hasFinish := false
		launchedAgentThisRound := false
		for _, tc := range assistantMessage.ToolCalls {
			if strings.HasPrefix(tc.Name, "Agent-") {
				launchedAgentThisRound = true
			}
		}
		for _, tm := range toolMessage {
			if tm.Content == "__WAIT_PENDING__" {
				hasWait = true
			}
			if tm.Content == "__FINISH_TASK_PENDING__" {
				hasFinish = true
			}
		}

		// Handle Tool-FinishTask: transitions to "决策结束" state and exits the loop.
		if hasFinish {
			finishResult := db.FinishTaskTool()
			for i := range toolMessage {
				if toolMessage[i].Content == "__FINISH_TASK_PENDING__" {
					toolMessage[i].Content = finishResult
					break
				}
			}
			// Persist tool messages and exit — brain loop stops until user chats again.
			for _, message := range toolMessage {
				db.memory.AddMessage(message)
			}
			if db.memoryFilePath != "" {
				_ = db.memory.SaveMemoryToFile(db.memoryFilePath)
			}
			return
		}

		// Handle Tool-Wait: enter blocking wait after adding tool messages.
		if hasWait {
			// Replace marker with actual result.
			for i := range toolMessage {
				if toolMessage[i].Content == "__WAIT_PENDING__" {
					toolMessage[i].Content = db.WaitTool()
					break
				}
			}
		}

		if len(toolMessage) == 0 {
			ready = false
		} else {
			ready = true
		}
		for _, message := range toolMessage {
			db.memory.AddMessage(message)
		}
		if err := db.memory.CompressIfNeeded(llm.GetResponsesClient("decision", "main_setting"), db.model); err != nil {
			misc.Error("decision", err.Error(), db.SubmitEventHandler)
		}
		if db.memoryFilePath != "" {
			_ = db.memory.SaveMemoryToFile(db.memoryFilePath)
		}

		// If Tool-Wait was called, block here until new events arrive.
		if hasWait {
			// Fallback: if all digital humans are idle while the brain chose to wait,
			// AND the brain did NOT launch any agents in this round,
			// inject a user message to nudge the brain into action instead of blocking.
			if !db.hasRunningAgents() && !launchedAgentThisRound {
				nudge := llm.Message{
					Role: llm.RoleUser,
					Content: "[System Nudge] You called Tool-Wait, but ALL digital humans are currently idle — no one is working. " +
						"Waiting will not produce any new events. You MUST take action NOW:\n" +
						"1. If there are unexplored vulnerability categories or code modules, schedule analysis agents with specific focus areas.\n" +
						"2. If there are exploitIdeas that need verification or combination into chains, schedule the appropriate agents.\n" +
						"3. If you are confident that the analysis is thorough and all objectives have been met, call Tool-FinishTask.\n" +
						"Do NOT call Tool-Wait again without first scheduling at least one agent.",
				}
				db.memory.AddMessage(nudge)
				misc.Debug("决策大脑兜底：所有数字人空闲且大脑选择等待，插入催促消息")
			} else {
				select {
				case <-db.notify:
				case <-db.done:
					return
				case <-time.After(60 * time.Second):
				}
			}
		}
	}
}

func (db *DecisionBrain) ToolCallRequest(msgList []llm.Message) (llm.Message, []llm.Message, error) {
	sendWS := func(typ string, data interface{}) {
		if typ == "BrainMessage" || typ == "BrainToolCall" {
			if m, ok := data.(map[string]interface{}); ok {
				db.AppendBrainFeed(typ, m)
			} else {
				db.AppendBrainFeed(typ, map[string]interface{}{"value": data})
			}
			// No longer push execution process to frontend.
			return
		}
		if db.webOutputChan == nil {
			return
		}
		msg := WebMsg{Type: typ, Data: data, ProjectName: db.projectName}
		if b, err := json.Marshal(msg); err == nil {
			db.trySendWS(string(b))
		}
	}
	agentTool := getAgentTools()
	tools := db.GetTools()
	tools = append(tools, agentTool...)
	count := 0
	cli := llm.GetResponsesClient("decision", "main_setting")
	var resp llm.Response
	var err error
	for {
		ctx, c := context.WithTimeout(db.ctx, time.Duration(600)*time.Second)
		defer c()
		resp, err = llm.RequestLLMWithOpts(cli, ctx, db.model, msgList, tools, llm.RequestLLMOpts{ProjectName: db.projectName, AgentLabel: "决策大脑"})
		if err == nil || count >= misc.GetMaxTryCount() {
			break
		}
		time.Sleep(time.Duration(5) * time.Second)
		count++
	}
	if err != nil {
		return llm.Message{}, nil, err
	}
	message := llm.ResponseToMessage(resp)
	db.pushTokenUsage()
	db.pushContextBreakdown()
	misc.Debug("决策大脑： %s 本次请求消息大小 %d", message.Content, db.memory.GetMsgSize())
	sendWS("BrainMessage", map[string]interface{}{
		"role":    message.Role,
		"content": message.Content,
	})
	// Detect <UserMessage> in brain reply and push to chat panel.
	if umsg := llm.ExtractTag(message.Content, "UserMessage"); umsg != "" {
		db.AppendChatMessage(ChatMessage{Role: "system", Text: umsg, Ts: time.Now().Format("15:04:05"), PersonaName: "决策大脑", AvatarFile: "system.png"})
		sendWS("UserMessage", map[string]interface{}{
			"persona_name": "决策大脑",
			"avatar_file":  "system.png",
			"agent_id":     "",
			"message":      umsg,
		})
	}
	var toolMessage []llm.Message
	for _, tc := range message.ToolCalls {
		var resultJSON string
		sendWS("BrainToolCall", map[string]interface{}{
			"stage":      "call",
			"toolCallID": tc.ID,
			"name":       tc.Name,
			"arguments":  tc.Arguments,
		})
		if strings.HasPrefix(tc.Name, "Agent-") {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
				errMsg := "parse arguments failed: " + err.Error()
				resultJSON = toolCalling.Fail(errMsg)
				sendWS("BrainToolCall", map[string]interface{}{
					"stage":      "result",
					"toolCallID": tc.ID,
					"name":       tc.Name,
					"error":      errMsg,
				})
				toolMessage = append(toolMessage, llm.Message{
					Role:       llm.RoleTool,
					Content:    resultJSON,
					ToolCallID: tc.ID,
				})
				continue
			}

			// Verifier/Report agents use exploit_idea_id/exploit_chain_id; others require task_content.
			isVerifier := strings.Contains(tc.Name, "Verifier")
			isReport := strings.Contains(tc.Name, "Report")
			if !isVerifier && !isReport {
				taskContent, ok := args["task_content"]
				if !ok || strings.TrimSpace(fmt.Sprint(taskContent)) == "" {
					errMsg := "task_content is required for Agent tools"
					resultJSON = toolCalling.Fail(errMsg)
					sendWS("BrainToolCall", map[string]interface{}{
						"stage":      "result",
						"toolCallID": tc.ID,
						"name":       tc.Name,
						"error":      errMsg,
					})
					toolMessage = append(toolMessage, llm.Message{
						Role:       llm.RoleTool,
						Content:    resultJSON,
						ToolCallID: tc.ID,
					})
					continue
				}
			}

			resultJSON = db.startAgent(tc.Name, tc.Arguments)
			sendWS("BrainToolCall", map[string]interface{}{
				"stage":      "result",
				"toolCallID": tc.ID,
				"name":       tc.Name,
				"result":     resultJSON,
			})
			toolMessage = append(toolMessage, llm.Message{
				Role:       llm.RoleTool,
				Content:    resultJSON,
				ToolCallID: tc.ID,
			})
		} else if tc.Name == "Tool-Wait" {
			resultJSON = "__WAIT_PENDING__"
			toolMessage = append(toolMessage, llm.Message{
				Role:       llm.RoleTool,
				Content:    resultJSON,
				ToolCallID: tc.ID,
			})
			sendWS("BrainToolCall", map[string]interface{}{
				"stage":      "result",
				"toolCallID": tc.ID,
				"name":       tc.Name,
				"result":     "Entering wait state...",
			})
		} else if tc.Name == "Tool-FinishTask" {
			// Guard: reject FinishTask if there are still active agents.
			activeIDs := db.getActiveAgentIDs()
			if len(activeIDs) > 0 {
				errMsg := fmt.Sprintf("Cannot finish task: %d agent(s) still running: %s. Wait for all agents to complete before calling Tool-FinishTask. Use Tool-Wait to wait.", len(activeIDs), strings.Join(activeIDs, ", "))
				resultJSON = toolCalling.Fail(errMsg)
				sendWS("BrainToolCall", map[string]interface{}{
					"stage":      "result",
					"toolCallID": tc.ID,
					"name":       tc.Name,
					"error":      errMsg,
				})
				toolMessage = append(toolMessage, llm.Message{
					Role:       llm.RoleTool,
					Content:    resultJSON,
					ToolCallID: tc.ID,
				})
				continue
			}
			// FinishTask must run outside the mutex because it blocks waiting for user.
			// Store a marker; the Start() loop will handle it after ToolCallRequest returns.
			resultJSON = "__FINISH_TASK_PENDING__"
			toolMessage = append(toolMessage, llm.Message{
				Role:       llm.RoleTool,
				Content:    resultJSON,
				ToolCallID: tc.ID,
			})
			sendWS("BrainToolCall", map[string]interface{}{
				"stage":      "result",
				"toolCallID": tc.ID,
				"name":       tc.Name,
				"result":     "Asking user for confirmation...",
			})
		} else if strings.HasPrefix(tc.Name, "Tool-") {
			//misc.Debug("决策大脑： function call: %s -- %s", tc.Name, tc.Arguments)
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
				sendWS("BrainToolCall", map[string]interface{}{
					"stage":      "result",
					"toolCallID": tc.ID,
					"name":       tc.Name,
					"error":      "parse arguments failed: " + err.Error(),
				})
				toolMessage = append(toolMessage, llm.Message{
					Role:       llm.RoleTool,
					Content:    toolCalling.Fail("parse arguments failed: " + err.Error()),
					ToolCallID: tc.ID,
				})
				continue
			}
			switch tc.Name {
			case "Tool-SynthesizeChainTool":
				resultJSON = db.SynthesizeChainTool(args)
			case "Tool-SearchExploitIdeaTool":
				resultJSON = db.SearchExploitIdeaTool(args)
			case "Tool-GetExploitIdeaByIdTool":
				resultJSON = db.GetExploitIdeaByIdTool(args)
			case "Tool-GetExploitChainByIdTool":
				resultJSON = db.GetExploitChainByIdTool(args)
			case "Tool-SendMessageToDigitalHuman":
				resultJSON = db.SendMessageToDigitalHumanTool(args)
			default:
				resultJSON = toolCalling.Fail("unknown tool: " + tc.Name)
			}
			sendWS("BrainToolCall", map[string]interface{}{
				"stage":      "result",
				"toolCallID": tc.ID,
				"name":       tc.Name,
				"result":     resultJSON,
			})
			toolMessage = append(toolMessage, llm.Message{
				Role:       llm.RoleTool,
				Content:    resultJSON,
				ToolCallID: tc.ID,
			})
		}
	}
	return message, toolMessage, nil
}
