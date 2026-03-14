package toolCalling

import (
	"AIxVuln/misc"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxMCPToolNameLength              = 64
	defaultGitHubCopilotMCPURL        = "https://api.githubcopilot.com/mcp/"
	defaultGitHubCopilotMCPToolPrefix = "MCP_GitHubCopilot"
)

type mcpServerConfig struct {
	Name              string                 `json:"name"`
	Type              string                 `json:"type,omitempty"`
	Enabled           *bool                  `json:"enabled,omitempty"`
	Command           string                 `json:"command,omitempty"`
	Args              []string               `json:"args,omitempty"`
	Env               map[string]interface{} `json:"env,omitempty"`
	WorkDir           string                 `json:"workdir,omitempty"`
	URL               string                 `json:"url,omitempty"`
	Headers           map[string]interface{} `json:"headers,omitempty"`
	ToolAllowlist     []string               `json:"tool_allowlist,omitempty"`
	ToolDenylist      []string               `json:"tool_denylist,omitempty"`
	ToolNamePrefix    string                 `json:"tool_name_prefix,omitempty"`
	ConnectTimeoutSec int                    `json:"connect_timeout_sec,omitempty"`
	CallTimeoutSec    int                    `json:"call_timeout_sec,omitempty"`
}

type mcpConnectedServer struct {
	cfg         mcpServerConfig
	session     *mcp.ClientSession
	callTimeout time.Duration
	mu          sync.Mutex
}

type mcpToolHandler struct {
	server         *mcpConnectedServer
	name           string
	remoteToolName string
	description    string
	parameters     map[string]interface{}
}

func (h *mcpToolHandler) Name() string {
	return h.name
}

func (h *mcpToolHandler) Description() string {
	return h.description
}

func (h *mcpToolHandler) Parameters() map[string]interface{} {
	return cloneMap(h.parameters)
}

func (h *mcpToolHandler) Execute(args map[string]interface{}) string {
	if h.server == nil || h.server.session == nil {
		return Fail("MCP session is not available")
	}
	callArgs := map[string]interface{}{}
	for k, v := range args {
		callArgs[k] = v
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.server.callTimeout)
	defer cancel()

	h.server.mu.Lock()
	res, err := h.server.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      h.remoteToolName,
		Arguments: callArgs,
	})
	h.server.mu.Unlock()
	if err != nil {
		return Fail(fmt.Sprintf("MCP tool call failed (%s/%s): %v", h.server.cfg.Name, h.remoteToolName, err))
	}

	result := map[string]interface{}{
		"server":  h.server.cfg.Name,
		"tool":    h.remoteToolName,
		"isError": res.IsError,
	}
	if content := mcpContentAsAny(res.Content); content != nil {
		result["content"] = content
	}
	if res.StructuredContent != nil {
		result["structuredContent"] = res.StructuredContent
	}
	if txt := strings.TrimSpace(extractMCPText(res.Content)); txt != "" {
		result["text"] = txt
	}
	if res.IsError {
		return Fail(result)
	}
	return Success(result)
}

var (
	mcpHandlersOnce sync.Once
	mcpHandlers     []ToolHandler

	githubCopilotMCPHandlersOnce sync.Once
	githubCopilotMCPHandlers     []ToolHandler
)

func GetMCPToolHandlers() []ToolHandler {
	mcpHandlersOnce.Do(func() {
		mcpHandlers = buildMCPHandlers()
	})
	out := make([]ToolHandler, len(mcpHandlers))
	copy(out, mcpHandlers)
	return out
}

func buildMCPHandlers() []ToolHandler {
	configs, err := readMCPServerConfigs()
	if err != nil {
		misc.Debug("MCP config parse failed: %v", err)
		return nil
	}
	return buildMCPHandlersFromConfigs(configs)
}

func buildMCPHandlersFromConfigs(configs []mcpServerConfig) []ToolHandler {
	if len(configs) == 0 {
		return nil
	}

	usedNames := make(map[string]struct{})
	out := make([]ToolHandler, 0)
	for _, cfg := range configs {
		handlers, err := connectMCPServer(cfg, usedNames)
		if err != nil {
			misc.Debug("MCP server %s init failed: %v", cfg.Name, err)
			continue
		}
		out = append(out, handlers...)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name() < out[j].Name()
	})
	if len(out) > 0 {
		misc.Debug("MCP loaded %d tools", len(out))
	}
	return out
}

func GetGitHubCopilotMCPToolHandlers() []ToolHandler {
	githubCopilotMCPHandlersOnce.Do(func() {
		githubCopilotMCPHandlers = buildMCPHandlersFromConfigs(readGitHubCopilotMCPServerConfigs())
	})
	out := make([]ToolHandler, len(githubCopilotMCPHandlers))
	copy(out, githubCopilotMCPHandlers)
	return out
}

func connectMCPServer(cfg mcpServerConfig, usedNames map[string]struct{}) ([]ToolHandler, error) {
	transport, err := buildMCPTransport(cfg)
	if err != nil {
		return nil, err
	}

	connectCtx, cancel := context.WithTimeout(context.Background(), cfg.connectTimeout())
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "AIxVuln", Version: "v1.0.0"}, nil)
	session, err := client.Connect(connectCtx, transport, nil)
	if err != nil {
		return nil, err
	}

	listCtx, listCancel := context.WithTimeout(context.Background(), cfg.connectTimeout())
	tools, err := listMCPTools(listCtx, session)
	listCancel()
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	if len(tools) == 0 {
		_ = session.Close()
		return nil, nil
	}

	allowSet := stringSet(cfg.ToolAllowlist)
	denySet := stringSet(cfg.ToolDenylist)
	server := &mcpConnectedServer{
		cfg:         cfg,
		session:     session,
		callTimeout: cfg.callTimeout(),
	}

	handlers := make([]ToolHandler, 0, len(tools))
	for _, tool := range tools {
		if tool == nil || strings.TrimSpace(tool.Name) == "" {
			continue
		}
		if len(allowSet) > 0 {
			if _, ok := allowSet[tool.Name]; !ok {
				continue
			}
		}
		if _, blocked := denySet[tool.Name]; blocked {
			continue
		}

		alias := buildMCPToolAlias(cfg, tool.Name, usedNames)
		desc := strings.TrimSpace(tool.Description)
		if desc == "" {
			desc = fmt.Sprintf("MCP tool %s from server %s", tool.Name, cfg.Name)
		} else {
			desc = fmt.Sprintf("[MCP %s/%s] %s", cfg.Name, tool.Name, desc)
		}
		parameters := normalizeMCPInputSchema(tool.InputSchema)
		handlers = append(handlers, &mcpToolHandler{
			server:         server,
			name:           alias,
			remoteToolName: tool.Name,
			description:    desc,
			parameters:     parameters,
		})
	}

	if len(handlers) == 0 {
		_ = session.Close()
		return nil, nil
	}
	return handlers, nil
}

func buildMCPTransport(cfg mcpServerConfig) (mcp.Transport, error) {
	switch cfg.Type {
	case "", "stdio":
		cmd := exec.Command(cfg.Command, cfg.Args...)
		if cfg.WorkDir != "" {
			cmd.Dir = cfg.WorkDir
		}
		if len(cfg.Env) > 0 {
			env := os.Environ()
			for k, v := range normalizeStringMap(cfg.Env) {
				env = append(env, k+"="+v)
			}
			cmd.Env = env
		}
		return &mcp.CommandTransport{Command: cmd}, nil
	case "streamable", "http":
		transport := &mcp.StreamableClientTransport{Endpoint: cfg.URL}
		headers := normalizeStringMap(cfg.Headers)
		if len(headers) > 0 {
			transport.HTTPClient = &http.Client{
				Transport: &headerInjectRoundTripper{
					base:    http.DefaultTransport,
					headers: headers,
				},
			}
		}
		return transport, nil
	default:
		return nil, fmt.Errorf("unsupported MCP transport type %q", cfg.Type)
	}
}

func listMCPTools(ctx context.Context, session *mcp.ClientSession) ([]*mcp.Tool, error) {
	var tools []*mcp.Tool
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, err
		}
		if tool != nil {
			tools = append(tools, tool)
		}
	}
	return tools, nil
}

func readMCPServerConfigs() ([]mcpServerConfig, error) {
	// `MCP_SERVERS` is intentionally no longer configurable via settings.
	// Keep default behavior as no extra dynamic MCP servers.
	return nil, nil
}

func readGitHubCopilotMCPServerConfigs() []mcpServerConfig {
	headerMap := map[string]interface{}{}
	auth := strings.TrimSpace(misc.GetConfigValueDefault("misc", "GITHUB_COPILOT_MCP_AUTHORIZATION", ""))
	if auth != "" {
		headerMap["Authorization"] = auth
	}
	cfg := mcpServerConfig{
		Name:              "githubcopilot",
		Type:              "streamable",
		URL:               defaultGitHubCopilotMCPURL,
		Headers:           headerMap,
		ToolNamePrefix:    defaultGitHubCopilotMCPToolPrefix,
		ConnectTimeoutSec: 10,
		CallTimeoutSec:    120,
	}
	return []mcpServerConfig{cfg}
}

func inferMCPServerName(cfg mcpServerConfig, idx int) string {
	if cfg.Command != "" {
		return sanitizeMCPToolName(filepath.Base(cfg.Command))
	}
	if cfg.URL != "" {
		if u, err := url.Parse(cfg.URL); err == nil {
			if host := strings.TrimSpace(u.Hostname()); host != "" {
				return sanitizeMCPToolName(host)
			}
		}
	}
	return fmt.Sprintf("mcp_server_%d", idx+1)
}

func buildMCPToolAlias(cfg mcpServerConfig, remoteToolName string, usedNames map[string]struct{}) string {
	prefix := cfg.ToolNamePrefix
	if prefix == "" {
		prefix = "MCP_" + cfg.Name
	}
	base := sanitizeMCPToolName(prefix + "_" + remoteToolName)
	base = shortenMCPToolName(base)
	name := base
	for i := 2; ; i++ {
		if _, exists := usedNames[name]; !exists {
			usedNames[name] = struct{}{}
			return name
		}
		name = shortenMCPToolName(fmt.Sprintf("%s_%d", base, i))
	}
}

func sanitizeMCPToolName(name string) string {
	if name == "" {
		return "MCP_Tool"
	}
	var b strings.Builder
	b.Grow(len(name))
	prevUnderscore := false
	for _, r := range name {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isDigit := r >= '0' && r <= '9'
		if isLetter || isDigit {
			b.WriteRune(r)
			prevUnderscore = false
			continue
		}
		if !prevUnderscore {
			b.WriteByte('_')
			prevUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		out = "MCP_Tool"
	}
	first := out[0]
	if first >= '0' && first <= '9' {
		out = "MCP_" + out
	}
	return out
}

func shortenMCPToolName(name string) string {
	if len(name) <= maxMCPToolNameLength {
		return name
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	suffix := fmt.Sprintf("%x", h.Sum32())
	keep := maxMCPToolNameLength - len(suffix) - 1
	if keep < 1 {
		keep = 1
	}
	return name[:keep] + "_" + suffix
}

func normalizeMCPInputSchema(raw any) map[string]interface{} {
	defaultSchema := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
	if raw == nil {
		return defaultSchema
	}

	var schema map[string]interface{}
	switch v := raw.(type) {
	case map[string]interface{}:
		schema = cloneMap(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return defaultSchema
		}
		if err := json.Unmarshal(b, &schema); err != nil {
			return defaultSchema
		}
	}

	if schema == nil {
		return defaultSchema
	}
	if t, ok := schema["type"].(string); !ok || strings.TrimSpace(t) == "" {
		schema["type"] = "object"
	}
	if _, ok := schema["properties"]; !ok {
		schema["properties"] = map[string]interface{}{}
	}
	return schema
}

func extractMCPText(contents []mcp.Content) string {
	lines := make([]string, 0, len(contents))
	for _, c := range contents {
		switch v := c.(type) {
		case *mcp.TextContent:
			if txt := strings.TrimSpace(v.Text); txt != "" {
				lines = append(lines, txt)
			}
		case *mcp.ResourceLink:
			if v.URI != "" {
				lines = append(lines, fmt.Sprintf("[resource] %s", v.URI))
			}
		case *mcp.EmbeddedResource:
			if v.Resource != nil {
				if txt := strings.TrimSpace(v.Resource.Text); txt != "" {
					lines = append(lines, txt)
				}
			}
		}
	}
	return strings.Join(lines, "\n")
}

func mcpContentAsAny(contents []mcp.Content) any {
	if len(contents) == 0 {
		return []interface{}{}
	}
	b, err := json.Marshal(contents)
	if err != nil {
		return nil
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

func stringSet(items []string) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		s := strings.TrimSpace(item)
		if s == "" {
			continue
		}
		out[s] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeStringMap(in map[string]interface{}) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = fmt.Sprint(v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (cfg mcpServerConfig) connectTimeout() time.Duration {
	sec := cfg.ConnectTimeoutSec
	if sec <= 0 {
		sec = 20
	}
	return time.Duration(sec) * time.Second
}

func (cfg mcpServerConfig) callTimeout() time.Duration {
	sec := cfg.CallTimeoutSec
	if sec <= 0 {
		sec = 120
	}
	return time.Duration(sec) * time.Second
}

type headerInjectRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (rt *headerInjectRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := rt.base
	if base == nil {
		base = http.DefaultTransport
	}
	cloned := req.Clone(req.Context())
	for k, v := range rt.headers {
		cloned.Header.Set(k, v)
	}
	return base.RoundTrip(cloned)
}
