package agents

import (
	"AIxVuln/llm"
	"AIxVuln/taskManager"
	"AIxVuln/toolCalling"
)

type ToolFactory func(*taskManager.Task) toolCalling.ToolHandler

type AgentBuildResult struct {
	Memory llm.Memory
	Client *toolCalling.ToolManager
}

func BuildAgentWithMemory(task *taskManager.Task, memory llm.Memory, systemPrompt string, toolFactories []ToolFactory) AgentBuildResult {
	if memory == nil {
		memory = llm.NewContextManager()
		memory.SetEventHandler(task.GetEventHandler())
		task.SetMemory(memory)
	}
	memory.SetSystemPrompt(&llm.SystemPromptX{SystemPrompt: systemPrompt, ContextId: task.GetTaskId()})

	client := toolCalling.NewToolManager()
	for _, factory := range toolFactories {
		if factory == nil {
			continue
		}
		client.Register(factory(task))
	}
	return AgentBuildResult{Memory: memory, Client: client}
}

func BuildAgent(task *taskManager.Task, systemPrompt string, toolFactories []ToolFactory) AgentBuildResult {
	return BuildAgentWithMemory(task, task.GetMemory(), systemPrompt, toolFactories)
}

func AnalyzeToolFactories() []ToolFactory {
	return []ToolFactory{
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewDetectLanguageTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewListSourceCodeTreeTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewSearchFileContentsByRegexTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewReadLinesFromFileTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewTaskListTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewGuidanceTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewIssueCandidateExploitIdeaTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewIssueTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewAgentFinishTool(task) },
	}
}

func OpsToolFactories() []ToolFactory {
	return []ToolFactory{
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewRunCommandTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewDetectLanguageTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewDockerRunTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewDockerLogsTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewDockerRemoveTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewDockerExecTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewEnvSaveTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewRunSQLTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewJavaEnvTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewPHPEnvTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewNodeEnvTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewPythonEnvTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewGolangEnvTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewMySQLEnvTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewRedisEnvTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewListSourceCodeTreeTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewSearchFileContentsByRegexTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewReadLinesFromFileTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewTaskListTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewGuidanceTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewIssueTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewAgentFinishTool(task) },
	}
}

func VerifierToolFactories() []ToolFactory {
	return []ToolFactory{
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewDetectLanguageTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewListSourceCodeTreeTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewSearchFileContentsByRegexTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewReadLinesFromFileTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewTaskListTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewGuidanceTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewRunCommandTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewSubmitExploitIdeaTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewSubmitExploitChainTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewRunPythonCodeTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewRunPHPCodeTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewRunSQLTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewDockerLogsTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewDockerDirScanTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewDockerFileReadTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewGetExploitIdeaByIdTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewGetExploitChainByIdTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewIssueTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewAgentFinishTool(task) },
	}
}

func OverviewToolFactories() []ToolFactory {
	return []ToolFactory{
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewDetectLanguageTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewListSourceCodeTreeTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewReadLinesFromFileTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewSearchFileContentsByRegexTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewAgentFinishTool(task) },
	}
}

func ReportToolFactories(reportType string) []ToolFactory {
	base := []ToolFactory{
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewListSourceCodeTreeTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewSearchFileContentsByRegexTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewReadLinesFromFileTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewIssueTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewGuidanceTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewReportVulnTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewGetExploitIdeaByIdTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewGetExploitChainByIdTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewAgentFinishTool(task) },
	}
	if reportType == "verifier" {
		base = append(base,
			func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewDockerLogsTool(task) },
			func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewRunPythonCodeTool(task) },
			func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewRunPHPCodeTool(task) },
			func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewRunCommandTool(task) },
			func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewDockerFileReadTool(task) },
			func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewDockerDirScanTool(task) },
			func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewRunSQLTool(task) },
		)
	}
	return base
}

func GeneralToolFactories() []ToolFactory {
	return []ToolFactory{
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewRunCommandTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewDetectLanguageTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewListSourceCodeTreeTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewSearchFileContentsByRegexTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler {
			return toolCalling.NewReadLinesFromFileTool(task)
		},
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewRunPythonCodeTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewRunPHPCodeTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewRunSQLTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewDockerLogsTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewDockerDirScanTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewDockerFileReadTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewTaskListTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewGuidanceTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewIssueTool(task) },
		func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewAgentFinishTool(task) },
	}
}
