package Web

import (
	"AIxVuln/ProjectManager"
	"AIxVuln/llm"
	"AIxVuln/misc"
	"AIxVuln/toolCalling"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const defaultAuditTaskContent = "尽可能的挖掘项目中的漏洞，侧重挖掘未授权情况下能完成的高危攻击链"

var projectNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func (s *Server) createProjectFromSourceDir(projectName, taskContent, sourceDir string) error {
	if err := validateProjectName(projectName); err != nil {
		return err
	}
	projectConfig := ProjectManager.ProjectConfig{ProjectName: projectName, SourceCodeDir: sourceDir, MsgChan: s.msgChan, TaskContent: taskContent}
	pm, err := ProjectManager.NewProjectManager(projectConfig)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.pms[projectName] = pm
	s.mu.Unlock()
	s.SaveProjectManagerToFile()
	return nil
}

func (s *Server) createProjectFromGitURL(ctx context.Context, projectName, taskContent, gitURL string) error {
	gitURL = strings.TrimSpace(gitURL)
	if gitURL == "" {
		return fmt.Errorf("请提供 Git 仓库地址")
	}

	absPath, err := filepath.Abs(misc.GetDataDir())
	if err != nil {
		return fmt.Errorf("获取DataDir失败: %w", err)
	}
	tempDir := filepath.Join(absPath, "temp", uuid.New().String())
	defer os.RemoveAll(tempDir)

	if err := os.MkdirAll(filepath.Dir(tempDir), 0755); err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", gitURL, tempDir)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Git clone 失败: %v\n%s", err, strings.TrimSpace(string(output)))
	}
	flattenSingleRootDirectory(tempDir)
	return s.createProjectFromSourceDir(projectName, taskContent, tempDir)
}

func flattenSingleRootDirectory(tempDir string) {
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return
	}
	dirs := 0
	files := 0
	singleDir := ""
	for _, e := range entries {
		if e.IsDir() {
			if e.Name() == ".git" {
				continue
			}
			dirs++
			singleDir = e.Name()
			continue
		}
		files++
	}
	if dirs != 1 || files != 0 {
		return
	}
	nested := filepath.Join(tempDir, singleDir)
	subEntries, err := os.ReadDir(nested)
	if err != nil {
		return
	}
	for _, se := range subEntries {
		src := filepath.Join(nested, se.Name())
		dst := filepath.Join(tempDir, se.Name())
		_ = os.Rename(src, dst)
	}
	_ = os.Remove(nested)
}

func validateProjectName(projectName string) error {
	matched, err := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, projectName)
	if err != nil {
		return fmt.Errorf("ProjectName只允许^[a-zA-Z0-9_-]+$")
	}
	if !matched {
		return fmt.Errorf("ProjectName只允许^[a-zA-Z0-9_-]+$")
	}
	return nil
}

type intelligentAddOpenSourceAuditReq struct {
	Query              string `json:"query"`
	MaxProjects        int    `json:"max_projects"`
	Language           string `json:"language"`
	StarsMin           int    `json:"stars_min"`
	StarsMax           int    `json:"stars_max"`
	UpdatedWithinDays  int    `json:"updated_within_days"`
	GitHubMCPMaxRounds int    `json:"github_mcp_max_rounds"`
	AutoStart          bool   `json:"auto_start"`
	AuditTaskContent   string `json:"audit_task_content"`
}

type intelligentRepoCandidate struct {
	FullName    string `json:"full_name"`
	CloneURL    string `json:"clone_url"`
	HtmlURL     string `json:"html_url"`
	Description string `json:"description,omitempty"`
	Language    string `json:"language,omitempty"`
	Stars       int    `json:"stars,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type scoutTraceItem struct {
	Time   string `json:"time"`
	Stage  string `json:"stage"`
	Detail string `json:"detail"`
}

type scoutDiagnostics struct {
	MCPToolCount  int              `json:"mcp_tool_count"`
	MCPToolNames  []string         `json:"mcp_tool_names,omitempty"`
	Rounds        int              `json:"rounds"`
	ToolCallCount int              `json:"tool_call_count"`
	LastError     string           `json:"last_error,omitempty"`
	Trace         []scoutTraceItem `json:"trace,omitempty"`
}

type intelligentAddProgress struct {
	Stage   string
	Detail  string
	Percent int
}

type intelligentAddProgressHandler func(p intelligentAddProgress)

func normalizeIntelligentAddOpenSourceAuditReq(req *intelligentAddOpenSourceAuditReq) error {
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		return fmt.Errorf("query is required")
	}
	if req.MaxProjects <= 0 {
		req.MaxProjects = 3
	}
	if req.MaxProjects > 10 {
		req.MaxProjects = 10
	}
	if req.StarsMin < 0 {
		req.StarsMin = 0
	}
	if req.StarsMax < 0 {
		req.StarsMax = 0
	}
	if req.StarsMin > 0 && req.StarsMax > 0 && req.StarsMin > req.StarsMax {
		return fmt.Errorf("stars_min cannot be greater than stars_max")
	}
	if req.GitHubMCPMaxRounds < 0 {
		req.GitHubMCPMaxRounds = 0
	}
	if req.GitHubMCPMaxRounds > 500 {
		req.GitHubMCPMaxRounds = 500
	}
	req.AuditTaskContent = strings.TrimSpace(req.AuditTaskContent)
	if req.AuditTaskContent == "" {
		req.AuditTaskContent = defaultAuditTaskContent
	}
	return nil
}

func (s *Server) runIntelligentAddOpenSourceAuditWorkflow(ctx context.Context, req intelligentAddOpenSourceAuditReq, progressCb intelligentAddProgressHandler) (map[string]interface{}, error) {
	emit := func(stage, detail string, percent int) {
		if progressCb == nil {
			return
		}
		progressCb(intelligentAddProgress{Stage: stage, Detail: detail, Percent: percent})
	}
	emit("init", "开始执行智能添加任务", 1)
	emit("scout", "正在使用通用 Agent 检索 GitHub 仓库", 5)
	existingProjects := s.listProjectNames()

	selected, scoutSummary, diag, err := runGeneralGitHubScout(ctx, req, existingProjects, func(stage, detail string) {
		emit("scout_"+stage, detail, 10)
	})
	if err != nil {
		return map[string]interface{}{
			"query":                 req.Query,
			"selected_repositories": []intelligentRepoCandidate{},
			"created_projects":      []map[string]interface{}{},
			"started_projects":      []string{},
			"queued_projects":       []map[string]interface{}{},
			"skipped_items":         []map[string]string{},
			"failed_items":          []map[string]string{},
			"auto_start":            req.AutoStart,
			"scout_summary":         "",
			"scout_diagnostics":     diag,
		}, err
	}
	if len(selected) == 0 {
		emit("done", "未检索到符合条件的仓库", 100)
		return map[string]interface{}{
			"query":                 req.Query,
			"selected_repositories": []intelligentRepoCandidate{},
			"created_projects":      []map[string]interface{}{},
			"started_projects":      []string{},
			"queued_projects":       []map[string]interface{}{},
			"skipped_items":         []map[string]string{},
			"failed_items":          []map[string]string{},
			"auto_start":            req.AutoStart,
			"scout_summary":         scoutSummary,
			"scout_diagnostics":     diag,
		}, nil
	}

	emit("create", fmt.Sprintf("检索完成，共 %d 个候选仓库，开始创建项目", len(selected)), 30)
	usedNames := make(map[string]struct{})
	createdProjects := make([]map[string]interface{}, 0, len(selected))
	startedProjects := make([]string, 0, len(selected))
	queuedProjects := make([]map[string]interface{}, 0, len(selected))
	skippedItems := make([]map[string]string, 0)
	failedItems := make([]map[string]string, 0)

	for i, repo := range selected {
		percent := 30
		if len(selected) > 0 {
			percent = 30 + int(float64(i)/float64(len(selected))*60)
		}
		emit("create_project", fmt.Sprintf("创建项目 %d/%d: %s", i+1, len(selected), repo.FullName), percent)
		canonicalProjectName := canonicalProjectNameForRepo(repo.FullName)
		if s.projectExists(canonicalProjectName) {
			msg := fmt.Sprintf("跳过重复仓库: %s（对应项目 %s 已存在）", repo.FullName, canonicalProjectName)
			skippedItems = append(skippedItems, map[string]string{
				"full_name":     repo.FullName,
				"project_name":  canonicalProjectName,
				"skip_reason":   "project already exists",
				"existing_name": canonicalProjectName,
			})
			emit("skip_duplicate", msg, percent)
			continue
		}
		projectName := s.allocateProjectNameForRepo(repo.FullName, usedNames)
		if err := s.createProjectFromGitURL(ctx, projectName, req.AuditTaskContent, repo.CloneURL); err != nil {
			failedItems = append(failedItems, map[string]string{
				"full_name": repo.FullName,
				"clone_url": repo.CloneURL,
				"error":     err.Error(),
			})
			emit("create_failed", fmt.Sprintf("创建失败: %s (%s)", repo.FullName, err.Error()), percent)
			continue
		}
		createdProjects = append(createdProjects, map[string]interface{}{
			"project_name": projectName,
			"repository":   repo,
		})
		emit("create_ok", fmt.Sprintf("创建成功: %s -> %s", repo.FullName, projectName), percent)
		if req.AutoStart {
			state, pos := s.enqueueProjectStart(projectName)
			startedProjects = append(startedProjects, projectName)
			queuedProjects = append(queuedProjects, map[string]interface{}{
				"project_name": projectName,
				"state":        state,
				"queue_pos":    pos,
				"message":      formatQueueStartMessage(state, pos),
			})
			emit("enqueue", fmt.Sprintf("%s: %s", projectName, formatQueueStartMessage(state, pos)), percent)
		}
	}
	emit("done", "智能添加任务执行完成", 100)
	return map[string]interface{}{
		"query":                 req.Query,
		"selected_repositories": selected,
		"created_projects":      createdProjects,
		"started_projects":      startedProjects,
		"queued_projects":       queuedProjects,
		"skipped_items":         skippedItems,
		"failed_items":          failedItems,
		"auto_start":            req.AutoStart,
		"scout_summary":         scoutSummary,
		"scout_diagnostics":     diag,
	}, nil
}

func (s *Server) intelligentAddOpenSourceAudit(c *gin.Context) {
	missing := misc.CheckRequiredConfig()
	if len(missing) > 0 {
		c.JSON(400, Fail("请先在「设置」中配置必填项: "+strings.Join(missing, ", ")))
		return
	}

	var req intelligentAddOpenSourceAuditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, Fail("invalid JSON: "+err.Error()))
		return
	}
	if err := normalizeIntelligentAddOpenSourceAuditReq(&req); err != nil {
		c.JSON(400, Fail(err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()

	result, err := s.runIntelligentAddOpenSourceAuditWorkflow(ctx, req, nil)
	if err != nil {
		c.JSON(500, Fail(map[string]interface{}{
			"message": err.Error(),
			"result":  result,
		}))
		return
	}
	c.JSON(200, Success(result))
}

func (s *Server) allocateProjectNameForRepo(fullName string, usedNames map[string]struct{}) string {
	base := canonicalProjectNameForRepo(fullName)
	candidate := base
	for i := 2; ; i++ {
		if _, used := usedNames[candidate]; !used {
			s.mu.RLock()
			_, exists := s.pms[candidate]
			s.mu.RUnlock()
			if !exists {
				usedNames[candidate] = struct{}{}
				return candidate
			}
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
}

func canonicalProjectNameForRepo(fullName string) string {
	base := sanitizeProjectNameBase(fullName)
	if base == "" {
		base = "gh-repo"
	}
	base = "gh-" + base
	if len(base) > 50 {
		base = strings.Trim(base[:50], "-_")
	}
	if base == "" {
		base = "gh-repo"
	}
	return base
}

func (s *Server) projectExists(projectName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.pms[projectName]
	return exists
}

func (s *Server) listProjectNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.pms))
	for name := range s.pms {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sanitizeProjectNameBase(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	s = projectNameSanitizer.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-_")
	return s
}

func runGeneralGitHubScout(ctx context.Context, req intelligentAddOpenSourceAuditReq, existingProjects []string, traceCb func(stage, detail string)) ([]intelligentRepoCandidate, string, *scoutDiagnostics, error) {
	diag := &scoutDiagnostics{Trace: make([]scoutTraceItem, 0, 32)}
	appendTrace := func(stage, detail string) {
		if len(diag.Trace) >= 80 {
			return
		}
		diag.Trace = append(diag.Trace, scoutTraceItem{
			Time:   time.Now().Format("15:04:05"),
			Stage:  stage,
			Detail: detail,
		})
		if traceCb != nil {
			traceCb(stage, detail)
		}
	}

	handlers := toolCalling.GetGitHubCopilotMCPToolHandlers()
	diag.MCPToolCount = len(handlers)
	diag.MCPToolNames = make([]string, 0, len(handlers))
	for _, h := range handlers {
		if h != nil {
			diag.MCPToolNames = append(diag.MCPToolNames, h.Name())
		}
	}
	appendTrace("init", fmt.Sprintf("加载 GitHub MCP 工具数量: %d", len(handlers)))
	if len(handlers) == 0 {
		err := fmt.Errorf("GitHub Copilot MCP 未连接可用工具。请在设置中检查 GITHUB_COPILOT_MCP_AUTHORIZATION")
		diag.LastError = err.Error()
		appendTrace("error", diag.LastError)
		return nil, "", diag, err
	}
	model := strings.TrimSpace(misc.GetConfigValueDefault("general", "MODEL", misc.GetConfigValueDefault("main_setting", "MODEL", "")))
	if model == "" {
		err := fmt.Errorf("未配置可用模型（general.MODEL/main_setting.MODEL）")
		diag.LastError = err.Error()
		appendTrace("error", diag.LastError)
		return nil, "", diag, err
	}
	appendTrace("init", fmt.Sprintf("使用模型: %s", model))
	cli := llm.GetResponsesClient("general", "main_setting")
	if cli == nil {
		err := fmt.Errorf("无法初始化 LLM client（general/main_setting）")
		diag.LastError = err.Error()
		appendTrace("error", diag.LastError)
		return nil, "", diag, err
	}

	toolMgr := toolCalling.NewToolManager()
	for _, t := range toolMgr.GetTools() {
		toolMgr.RemoveTool(t.Name)
	}
	toolMgr.RegisterHandlers(handlers...)

	sysPrompt := `You are a general-purpose digital human for GitHub open-source repository scouting.
You can use GitHub Copilot MCP tools to search and inspect repositories.
Your goal is to select suitable open-source projects for security audit tasks.
You MUST finally output strict JSON only (no markdown, no extra text).`

	userPrompt := buildScoutUserPrompt(req, existingProjects)
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: sysPrompt},
		{Role: llm.RoleUser, Content: userPrompt},
	}

	if req.GitHubMCPMaxRounds > 0 {
		appendTrace("config", fmt.Sprintf("GitHub MCP 最大轮次限制: %d", req.GitHubMCPMaxRounds))
	} else {
		appendTrace("config", "GitHub MCP 轮次限制: 无（受任务超时控制）")
	}

	for round := 0; ; round++ {
		if req.GitHubMCPMaxRounds > 0 && round >= req.GitHubMCPMaxRounds {
			break
		}
		if err := ctx.Err(); err != nil {
			diag.LastError = err.Error()
			appendTrace("error", "任务上下文结束: "+diag.LastError)
			return nil, "", diag, err
		}
		diag.Rounds = round + 1
		appendTrace("llm_round_start", fmt.Sprintf("第 %d 轮请求", round+1))
		assistantMessage, toolMessages, err := toolMgr.ToolCallRequestWithLabel(
			ctx,
			cli,
			messages,
			model,
			"GeneralOpenSourceScout",
			"GeneralOpenSourceScout",
		)
		if err != nil {
			diag.LastError = err.Error()
			appendTrace("error", "LLM 请求失败: "+diag.LastError)
			return nil, "", diag, err
		}
		if len(assistantMessage.ToolCalls) > 0 {
			diag.ToolCallCount += len(assistantMessage.ToolCalls)
			names := make([]string, 0, len(assistantMessage.ToolCalls))
			for _, tc := range assistantMessage.ToolCalls {
				names = append(names, tc.Name)
			}
			appendTrace("tool_calls", fmt.Sprintf("第 %d 轮调用工具 %d 次: %s", round+1, len(assistantMessage.ToolCalls), strings.Join(names, ", ")))
		} else {
			appendTrace("assistant_reply", fmt.Sprintf("第 %d 轮无工具调用，内容长度=%d", round+1, len([]rune(strings.TrimSpace(assistantMessage.Content)))))
		}

		messages = append(messages, assistantMessage)
		if len(toolMessages) > 0 {
			appendTrace("tool_results", fmt.Sprintf("第 %d 轮返回工具结果 %d 条", round+1, len(toolMessages)))
			messages = append(messages, toolMessages...)
			continue
		}

		if len(assistantMessage.ToolCalls) > 0 {
			messages = append(messages, llm.Message{
				Role:    llm.RoleUser,
				Content: "Tool call was requested but no tool result was produced. Please continue and retry with available MCP tools.",
			})
			continue
		}

		content := strings.TrimSpace(assistantMessage.Content)
		if content == "" {
			messages = append(messages, llm.Message{Role: llm.RoleUser, Content: "回复为空。请输出最终 JSON。"})
			continue
		}

		repos, summary, parseErr := parseScoutResult(content)
		if parseErr == nil {
			if len(repos) == 0 {
				appendTrace("done_empty", "解析成功，但候选仓库为空，结束检索")
				return []intelligentRepoCandidate{}, summary, diag, nil
			}
			repos = filterReposByStarsRange(repos, req.StarsMin, req.StarsMax)
			if len(repos) == 0 {
				appendTrace("done_empty", "候选仓库未满足 Star 范围过滤条件，结束检索（0结果）")
				return []intelligentRepoCandidate{}, summary, diag, nil
			}
			if len(repos) > req.MaxProjects {
				repos = repos[:req.MaxProjects]
			}
			appendTrace("done", fmt.Sprintf("解析成功，命中仓库 %d 个", len(repos)))
			return repos, summary, diag, nil
		}
		diag.LastError = parseErr.Error()
		appendTrace("parse_error", "JSON 解析失败: "+diag.LastError)
		messages = append(messages, llm.Message{
			Role:    llm.RoleUser,
			Content: "上一条回复未能解析为有效 JSON。请严格按指定 JSON schema 重新输出，不要包含 markdown/code fence/注释。",
		})
	}

	err := fmt.Errorf("智能检索未在限定轮次内生成可解析结果（github_mcp_max_rounds=%d）", req.GitHubMCPMaxRounds)
	diag.LastError = err.Error()
	appendTrace("error", diag.LastError)
	return nil, "", diag, err
}

func buildScoutUserPrompt(req intelligentAddOpenSourceAuditReq, existingProjects []string) string {
	constraints := []string{
		fmt.Sprintf("query: %s", req.Query),
		fmt.Sprintf("max_projects: %d", req.MaxProjects),
	}
	if strings.TrimSpace(req.Language) != "" {
		constraints = append(constraints, "language: "+strings.TrimSpace(req.Language))
	}
	if req.StarsMin > 0 {
		constraints = append(constraints, fmt.Sprintf("stars_min: %d", req.StarsMin))
	}
	if req.StarsMax > 0 {
		constraints = append(constraints, fmt.Sprintf("stars_max: %d", req.StarsMax))
	}
	if req.UpdatedWithinDays > 0 {
		constraints = append(constraints, fmt.Sprintf("updated_within_days: %d", req.UpdatedWithinDays))
	}

	return `请使用 GitHub MCP 工具在 GitHub 上查找适合安全审计的开源项目，并给出推荐结果。
要求：
1. 优先选择真实维护中的开源项目，避免明显空仓库或样例仓库。
2. 尽量让技术栈多样化（如果可行）。
3. 输出数量不超过 max_projects。
4. 每个项目都必须包含 clone_url（可直接 git clone）和 full_name（owner/repo）。
5. 每个项目都必须包含 stars（整数），并满足给定的 stars_min / stars_max 范围（若给定）。
6. 必须避开已有项目，禁止推荐“当前已有项目列表”中的项目。
7. 对每个候选仓库，先用规则转换出标准项目名：` + "`gh-` + lower(full_name).replace('/', '-')" + `（并做常规清洗）；若该名称已在“当前已有项目列表”中，必须跳过。
8. 若确实找不到符合条件的仓库，可返回空数组："repositories": []，并在 summary 说明原因。

筛选条件：
` + strings.Join(constraints, "\n") + `

当前已有项目列表（必须避开）：
` + formatExistingProjectsForPrompt(existingProjects) + `

最终只输出 JSON，严格符合以下结构：
{
  "summary": "一句中文总结",
  "repositories": [
    {
      "full_name": "owner/repo",
      "clone_url": "https://github.com/owner/repo.git",
      "html_url": "https://github.com/owner/repo",
      "description": "仓库简介",
      "language": "主要语言",
      "stars": 123,
      "reason": "为什么适合审计"
    }
  ]
}`
}

func formatExistingProjectsForPrompt(existingProjects []string) string {
	if len(existingProjects) == 0 {
		return "(empty)"
	}
	maxShow := 200
	show := existingProjects
	truncated := false
	if len(show) > maxShow {
		show = show[:maxShow]
		truncated = true
	}
	if !truncated {
		return strings.Join(show, "\n")
	}
	return strings.Join(show, "\n") + fmt.Sprintf("\n... (truncated, total=%d)", len(existingProjects))
}

func parseScoutResult(content string) ([]intelligentRepoCandidate, string, error) {
	candidates := extractJSONCandidates(content)
	var lastErr error
	for _, c := range candidates {
		result, err := parseScoutResultJSON(c)
		if err == nil {
			return result.Repositories, result.Summary, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no JSON object found")
	}
	return nil, "", lastErr
}

type scoutParsedResult struct {
	Summary      string
	Repositories []intelligentRepoCandidate
	RawCount     int
}

func parseScoutResultJSON(raw string) (*scoutParsedResult, error) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, err
	}

	summary := strings.TrimSpace(stringValue(data["summary"]))
	repoAny := data["repositories"]
	if repoAny == nil {
		repoAny = data["repos"]
	}
	if repoAny == nil {
		repoAny = data["items"]
	}
	if repoAny == nil {
		repoAny = data["list"]
	}
	if repoAny == nil {
		repoAny = data["results"]
	}
	repoList, ok := repoAny.([]interface{})
	if !ok {
		return nil, fmt.Errorf("repositories is not an array")
	}

	result := make([]intelligentRepoCandidate, 0, len(repoList))
	seen := make(map[string]struct{})
	for _, one := range repoList {
		obj, ok := one.(map[string]interface{})
		if !ok {
			continue
		}
		repo := normalizeRepoCandidate(obj)
		if repo.FullName == "" || repo.CloneURL == "" {
			continue
		}
		if _, exists := seen[repo.FullName]; exists {
			continue
		}
		seen[repo.FullName] = struct{}{}
		result = append(result, repo)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Stars == result[j].Stars {
			return result[i].FullName < result[j].FullName
		}
		return result[i].Stars > result[j].Stars
	})
	return &scoutParsedResult{Summary: summary, Repositories: result, RawCount: len(repoList)}, nil
}

func normalizeRepoCandidate(obj map[string]interface{}) intelligentRepoCandidate {
	fullName := firstNonEmptyString(obj,
		"full_name", "fullName", "repo_full_name", "repository", "repository_name", "repo_name", "repo", "name", "owner_repo",
		"仓库", "仓库名", "项目", "项目名")
	cloneURL := firstNonEmptyString(obj,
		"clone_url", "cloneUrl", "git_url", "repository_clone_url", "clone", "git", "ssh_url", "ssh", "clone_address",
		"克隆地址", "仓库克隆地址")
	htmlURL := firstNonEmptyString(obj,
		"html_url", "htmlUrl", "url", "web_url", "repository_url", "repo_url", "homepage", "web", "link",
		"仓库地址", "链接")

	if fullName == "" {
		if inferred := inferRepoFullNameFromURL(cloneURL); inferred != "" {
			fullName = inferred
		} else if inferred := inferRepoFullNameFromURL(htmlURL); inferred != "" {
			fullName = inferred
		}
	}
	if cloneURL == "" && fullName != "" {
		cloneURL = "https://github.com/" + fullName + ".git"
	}
	if htmlURL == "" && fullName != "" {
		htmlURL = "https://github.com/" + fullName
	}

	return intelligentRepoCandidate{
		FullName:    strings.TrimSpace(fullName),
		CloneURL:    strings.TrimSpace(cloneURL),
		HtmlURL:     strings.TrimSpace(htmlURL),
		Description: firstNonEmptyString(obj, "description", "desc", "summary", "简介", "描述"),
		Language:    firstNonEmptyString(obj, "language", "main_language", "lang", "主要语言"),
		Stars:       firstNonZeroInt(obj, "stars", "star", "stars_count", "stargazers_count", "stargazers", "星标"),
		Reason:      firstNonEmptyString(obj, "reason", "selection_reason", "why", "选择理由", "推荐理由"),
	}
}

func inferRepoFullNameFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(rawURL), "git@github.com:") {
		part := strings.TrimSpace(strings.TrimPrefix(rawURL, "git@github.com:"))
		part = strings.TrimSuffix(part, ".git")
		parts := strings.Split(part, "/")
		if len(parts) >= 2 {
			owner := strings.TrimSpace(parts[0])
			repo := strings.TrimSpace(parts[1])
			if owner != "" && repo != "" {
				return owner + "/" + repo
			}
		}
	}
	rawURL = strings.TrimSuffix(rawURL, ".git")
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if !strings.EqualFold(u.Hostname(), "github.com") {
		return ""
	}
	p := strings.Trim(u.Path, "/")
	parts := strings.Split(p, "/")
	if len(parts) < 2 {
		return ""
	}
	owner := strings.TrimSpace(parts[0])
	repo := strings.TrimSpace(parts[1])
	if owner == "" || repo == "" {
		return ""
	}
	return owner + "/" + repo
}

func firstNonEmptyString(obj map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := obj[k]; ok {
			s := strings.TrimSpace(stringValue(v))
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func firstNonZeroInt(obj map[string]interface{}, keys ...string) int {
	for _, k := range keys {
		if v, ok := obj[k]; ok {
			n := intValue(v)
			if n > 0 {
				return n
			}
		}
	}
	return 0
}

func stringValue(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	default:
		if x == nil {
			return ""
		}
		b, _ := json.Marshal(x)
		return strings.Trim(string(b), `"`)
	}
}

func intValue(v interface{}) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case int32:
		return int(x)
	case uint64:
		return int(x)
	case uint32:
		return int(x)
	case uint:
		return int(x)
	case float64:
		return int(x)
	case float32:
		return int(x)
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	default:
		return 0
	}
}

func extractJSONCandidates(text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	candidates := make([]string, 0, 4)
	seen := make(map[string]struct{})
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, exists := seen[v]; exists {
			return
		}
		seen[v] = struct{}{}
		candidates = append(candidates, v)
	}

	add(trimmed)
	if strings.Contains(trimmed, "```") {
		parts := strings.Split(trimmed, "```")
		for i := 1; i < len(parts); i += 2 {
			segment := strings.TrimSpace(parts[i])
			if strings.HasPrefix(strings.ToLower(segment), "json") {
				segment = strings.TrimSpace(segment[4:])
			}
			add(segment)
		}
	}
	first := strings.Index(trimmed, "{")
	last := strings.LastIndex(trimmed, "}")
	if first >= 0 && last > first {
		add(trimmed[first : last+1])
	}
	return candidates
}

func filterReposByStarsRange(repos []intelligentRepoCandidate, starsMin, starsMax int) []intelligentRepoCandidate {
	if starsMin <= 0 && starsMax <= 0 {
		return repos
	}
	filtered := make([]intelligentRepoCandidate, 0, len(repos))
	for _, repo := range repos {
		if !repoMatchesStarsRange(repo.Stars, starsMin, starsMax) {
			continue
		}
		filtered = append(filtered, repo)
	}
	return filtered
}

func repoMatchesStarsRange(stars, starsMin, starsMax int) bool {
	if starsMin <= 0 && starsMax <= 0 {
		return true
	}
	if stars <= 0 {
		return false
	}
	if starsMin > 0 && stars < starsMin {
		return false
	}
	if starsMax > 0 && stars > starsMax {
		return false
	}
	return true
}
