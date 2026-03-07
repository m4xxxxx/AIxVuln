package dockerManager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	image2 "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

func NewDockerManager() *DockerManager {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalln(err)
	}
	return &DockerManager{cli: cli}
}

type PortMapping struct {
	HostPort      string `json:"host_port"`      // 主机端口
	ContainerPort string `json:"container_port"` // 容器端口
	Protocol      string `json:"protocol"`       // 协议 (tcp/udp)
}

type DockerManager struct {
	cli                   *client.Client
	containerEventHandler func(infoJson string)
}

type RunConfig struct {
	Image      string            // 镜像名称
	Name       string            // 容器名称
	Cmd        []string          // 运行命令
	Ports      map[string]string // 端口映射 主机端口:容器端口
	Volumes    map[string]string // 卷映射 主机路径:容器路径
	Env        []string          // 环境变量
	AutoRemove bool              // 自动删除
	WorkingDir string            // 容器工作目录
}

type RunResult struct {
	ContainerID string        `json:"container_id"`
	StatusCode  int64         `json:"-"`
	PortMap     []PortMapping `json:"-"`
	IPAddress   string        `json:"ip_address"`
}

func (dm *DockerManager) SetEventHandler(handler func(infoJson string)) {
	dm.containerEventHandler = handler
}

func (dm *DockerManager) DockerRun(ctx context.Context, config *RunConfig) (*RunResult, error) {
	resolvedImage, err := pullImageIfNotExists(ctx, dm.cli, config.Image)
	if err != nil {
		return nil, fmt.Errorf("拉取镜像失败: %w", err)
	}
	containerConfig := &container.Config{
		Image:      resolvedImage,
		Cmd:        config.Cmd,
		Env:        config.Env,
		Tty:        false,
		OpenStdin:  false,
		WorkingDir: config.WorkingDir,
	}

	hostConfig := &container.HostConfig{
		AutoRemove: config.AutoRemove,
	}

	if len(config.Ports) > 0 {
		portBindings := nat.PortMap{}
		exposedPorts := nat.PortSet{}

		for hostPort, containerPort := range config.Ports {
			natPort, err := nat.NewPort("tcp", containerPort)
			if err != nil {
				return nil, fmt.Errorf("无效的端口: %s", containerPort)
			}

			exposedPorts[natPort] = struct{}{}
			portBindings[natPort] = []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: hostPort,
				},
			}
		}

		containerConfig.ExposedPorts = exposedPorts
		hostConfig.PortBindings = portBindings
	}

	if len(config.Volumes) > 0 {
		var mounts []mount.Mount
		for source, target := range config.Volumes {
			mounts = append(mounts, mount.Mount{
				Type:   mount.TypeBind,
				Source: source,
				Target: target,
			})
		}
		hostConfig.Mounts = mounts
	}

	resp, err := dm.cli.ContainerCreate(
		ctx,
		containerConfig,
		hostConfig,
		nil,
		nil,
		config.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("创建容器失败: %w", err)
	}
	containerID := resp.ID
	if len(containerID) > 12 {
		containerID = containerID[:12]
	}
	result := &RunResult{
		ContainerID: containerID,
	}

	if err := dm.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("启动容器失败: %w", err)
	}
	ip, portMap, err := GetContainerIPAndPortMap(dm.cli, resp.ID)
	result.IPAddress = ip
	result.PortMap = portMap
	if dm.containerEventHandler != nil {
		info := make(map[string]any)
		info["type"] = "Create"
		info["image"] = config.Image
		info["containerId"] = containerID
		info["containerIP"] = ip
		var portMapStr []string
		for _, d := range portMap {
			if d.HostPort != "" {
				portMapStr = append(portMapStr, fmt.Sprintf("%s", d.HostPort))
			}
		}
		info["webPort"] = portMapStr
		js, _ := json.Marshal(info)
		dm.containerEventHandler(string(js))
	}
	return result, nil
}

func (dm *DockerManager) DockerExec(containerID string, cmd []string, timeout int16) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	execConfig := container.ExecOptions{
		Cmd:          cmd,
		Tty:          false,
		ConsoleSize:  (*[2]uint)([]uint{uint(0), uint(0)}),
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := dm.cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("创建 exec 失败: %w", err)
	}

	resp, err := dm.cli.ContainerExecAttach(ctx, execID.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", fmt.Errorf("连接 exec 失败: %w", err)
	}
	defer resp.Close()

	if err := dm.cli.ContainerExecStart(ctx, execID.ID, container.ExecStartOptions{}); err != nil {
		return "", fmt.Errorf("启动 exec 失败: %w", err)
	}

	// 使用带超时的读取
	var output []byte
	done := make(chan error, 1)

	go func() {
		var err error
		output, err = io.ReadAll(resp.Reader)
		done <- err
	}()

	select {
	case <-ctx.Done():
		// 超时发生
		return "", fmt.Errorf("命令执行超时 (%v 秒)", timeout)
	case err := <-done:
		if err != nil {
			return "", fmt.Errorf("读取输出失败: %w", err)
		}
	}

	out := string(output)
	if len(out) >= 8 {
		out = out[8:] // 去掉 Docker 的前缀（通常是 8 字节的头部信息）
	}
	return out, nil
}

func (dm *DockerManager) DockerRemove(containerID string) error {
	ctx := context.Background()

	// 设置强制删除选项
	options := container.RemoveOptions{
		Force:         true,
		RemoveVolumes: false,
		RemoveLinks:   false,
	}
	err := dm.cli.ContainerRemove(ctx, containerID, options)
	if err != nil {
		return fmt.Errorf("删除容器失败: %w", err)
	}

	if dm.containerEventHandler != nil {
		info := make(map[string]string)
		info["type"] = "Remove"
		info["containerId"] = containerID
		js, _ := json.Marshal(info)
		dm.containerEventHandler(string(js))
	}

	return nil
}

func (dm *DockerManager) DockerLogs(containerID string) (string, error) {
	ctx := context.Background()

	// 设置日志选项
	options := container.LogsOptions{
		ShowStdout: true,  // 显示标准输出
		ShowStderr: true,  // 显示标准错误
		Follow:     false, // 不持续跟踪
		Tail:       "all", // 显示所有日志
		Timestamps: false, // 不显示时间戳
	}

	// 获取日志
	reader, err := dm.cli.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return "", fmt.Errorf("获取日志失败: %w", err)
	}
	defer reader.Close()

	// 读取日志内容
	output, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("读取日志失败: %w", err)
	}

	// Docker 日志有特殊的格式，需要使用 stdcopy 处理
	var stdout, stderr bytes.Buffer
	_, err = stdcopy.StdCopy(&stdout, &stderr, bytes.NewReader(output))
	if err != nil {
		// 如果处理失败，直接返回原始内容
		return string(output), nil
	}

	// 合并 stdout 和 stderr
	result := stdout.String()
	if stderr.Len() > 0 {
		result += "\n" + stderr.String()
	}

	return result, nil
}

func (dm *DockerManager) DockerPs() (string, error) {
	containers, err := dm.cli.ContainerList(context.Background(), container.ListOptions{
		All: true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list containers: %v", err)
	}

	// 使用 strings.Builder 构建结果字符串
	var result strings.Builder

	// 添加表头
	result.WriteString("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES\n")

	for _, container := range containers {
		// 截取容器ID前12位
		containerID := container.ID
		if len(containerID) > 12 {
			containerID = containerID[:12]
		}

		// 获取容器名称（移除开头的斜杠）
		var name string
		if len(container.Names) > 0 {
			name = strings.TrimPrefix(container.Names[0], "/")
		}

		// 格式化命令（限制长度）
		command := container.Command
		if len(command) > 30 {
			command = command[:27] + "..."
		}

		// 格式化时间
		createdStr := formatContainerTime(container.Created)

		// 格式化端口
		portsStr := formatPorts(container.Ports)

		// 添加行数据
		result.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			containerID,
			container.Image,
			command,
			createdStr,
			container.Status,
			portsStr,
			name,
		))
	}

	return result.String(), nil
}

// 辅助函数：格式化容器创建时间
func formatContainerTime(created int64) string {
	createdTime := time.Unix(created, 0)
	duration := time.Since(createdTime)

	if duration.Hours() > 24*30 {
		months := int(duration.Hours() / (24 * 30))
		return fmt.Sprintf("%d months ago", months)
	} else if duration.Hours() > 24 {
		days := int(duration.Hours() / 24)
		return fmt.Sprintf("%d days ago", days)
	} else if duration.Hours() >= 1 {
		return fmt.Sprintf("%.0f hours ago", duration.Hours())
	} else if duration.Minutes() >= 1 {
		return fmt.Sprintf("%.0f minutes ago", duration.Minutes())
	}
	return fmt.Sprintf("%.0f seconds ago", duration.Seconds())
}

// 辅助函数：格式化端口信息
func formatPorts(ports []types.Port) string {
	if len(ports) == 0 {
		return ""
	}

	var result []string
	for _, port := range ports {
		if port.IP != "" {
			result = append(result, fmt.Sprintf("%s:%d->%d/%s",
				port.IP, port.PublicPort, port.PrivatePort, port.Type))
		} else {
			result = append(result, fmt.Sprintf("%d/%s", port.PrivatePort, port.Type))
		}
	}
	return strings.Join(result, ", ")
}

// 拉取镜像（如果不存在）
func pullImageIfNotExists(ctx context.Context, cli *client.Client, image string) (string, error) {
	aliases := imageAliases(image)
	// 检查镜像是否存在
	images, err := cli.ImageList(ctx, image2.ListOptions{})
	if err != nil {
		return "", err
	}

	for _, img := range images {
		for _, tag := range img.RepoTags {
			for _, alias := range aliases {
				if tag == alias || strings.HasPrefix(tag, alias+":") {
					return alias, nil // 镜像已存在
				}
			}
		}
	}

	// 拉取镜像（优先尝试命名空间镜像）
	for _, alias := range aliases {
		out, pullErr := cli.ImagePull(ctx, alias, image2.PullOptions{})
		if pullErr != nil {
			continue
		}
		defer out.Close()

		// 等待拉取完成
		if _, copyErr := io.Copy(io.Discard, out); copyErr != nil {
			return "", copyErr
		}
		return alias, nil
	}

	return "", fmt.Errorf("镜像不存在且拉取失败: %s (尝试别名: %s)", image, strings.Join(aliases, ", "))
}

func imageAliases(image string) []string {
	seen := map[string]bool{}
	aliases := make([]string, 0, 2)
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		aliases = append(aliases, v)
	}

	add(image)
	if !strings.Contains(image, "/") {
		add("aixvuln/" + image)
	} else if strings.HasPrefix(image, "aixvuln/") {
		base := strings.TrimPrefix(image, "aixvuln/")
		if !strings.Contains(base, "/") {
			add(base)
		}
	}
	return aliases
}

// 简便的 Run 函数，支持选项模式，支持超时（秒，0为永不超时）
func (dm *DockerManager) Run(image string, cmd []string, timeoutSec int16, opts ...Option) (*RunResult, error) {
	config := &RunConfig{
		Image: image,
		Cmd:   cmd,
	}

	// 应用选项
	for _, opt := range opts {
		opt(config)
	}
	if timeoutSec > 0 {
		ctx, c := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
		defer c()
		return dm.DockerRun(ctx, config)
	}

	r, e := dm.DockerRun(context.Background(), config)
	if e != nil {
		return nil, e
	}
	return r, nil

}

// Option 配置选项
type Option func(*RunConfig)

// 设置容器名称
func SetName(name string) Option {
	return func(c *RunConfig) {
		c.Name = name
	}
}

// 设置端口映射
func SetPort(hostPort, containerPort string) Option {
	return func(c *RunConfig) {
		if c.Ports == nil {
			c.Ports = make(map[string]string)
		}
		c.Ports[hostPort] = containerPort
	}
}

// 设置多个端口映射
func SetPorts(ports map[string]string) Option {
	return func(c *RunConfig) {
		if c.Ports == nil {
			c.Ports = make(map[string]string)
		}
		for hostPort, containerPort := range ports {
			c.Ports[hostPort] = containerPort
		}
	}
}

// 设置卷映射
func SetVolume(hostPath, containerPath string) Option {
	return func(c *RunConfig) {
		if c.Volumes == nil {
			c.Volumes = make(map[string]string)
		}
		c.Volumes[hostPath] = containerPath
	}
}

// 设置多个卷映射
func SetVolumes(volumes map[string]string) Option {
	return func(c *RunConfig) {
		if c.Volumes == nil {
			c.Volumes = make(map[string]string)
		}
		for hostPath, containerPath := range volumes {
			c.Volumes[hostPath] = containerPath
		}
	}
}

// 设置环境变量
func SetEnv(key, value string) Option {
	return func(c *RunConfig) {
		c.Env = append(c.Env, fmt.Sprintf("%s=%s", key, value))
	}
}

// 设置多个环境变量
func SetEnvs(envs map[string]string) Option {
	return func(c *RunConfig) {
		for key, value := range envs {
			c.Env = append(c.Env, fmt.Sprintf("%s=%s", key, value))
		}
	}
}

// 设置自动删除
func SetAutoRemove() Option {
	return func(c *RunConfig) {
		c.AutoRemove = true
	}
}

// 设置容器工作目录
func SetWorkingDir(dir string) Option {
	return func(c *RunConfig) {
		c.WorkingDir = dir
	}
}

// GetContainerIPAndPortMap 获取容器IP和端口映射（支持-P随机端口）
func GetContainerIPAndPortMap(cli *client.Client, containerID string) (string, []PortMapping, error) {
	ctx := context.Background()

	// 获取容器详细信息
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", nil, fmt.Errorf("获取容器信息失败: %w", err)
	}

	// 获取IP地址
	ipAddress := ""
	if containerJSON.NetworkSettings != nil {
		for _, network := range containerJSON.NetworkSettings.Networks {
			if network.IPAddress != "" {
				ipAddress = network.IPAddress
				break
			}
		}
	}

	// 获取端口映射
	portMappings := getPortMappings(containerJSON)

	return ipAddress, portMappings, nil
}

func getPortMappings(container types.ContainerJSON) []PortMapping {
	var mappings []PortMapping

	// 方法1：从 NetworkSettings.Ports 获取（对于随机端口可能不完整）
	if container.NetworkSettings != nil {
		for portBindings, bindings := range container.NetworkSettings.Ports {
			if len(bindings) > 0 {
				for _, binding := range bindings {
					mapping := PortMapping{
						ContainerPort: portBindings.Port(),
						HostPort:      binding.HostPort,
						Protocol:      portBindings.Proto(),
					}
					mappings = append(mappings, mapping)
				}
			} else {
				// 对于 -p :8080 这种格式，bindings 可能为空
				// 需要从 HostConfig.PortBindings 获取
				mapping := PortMapping{
					ContainerPort: portBindings.Port(),
					Protocol:      portBindings.Proto(),
				}
				mappings = append(mappings, mapping)
			}
		}
	}

	// 方法2：从 HostConfig.PortBindings 获取完整映射
	if container.HostConfig != nil && container.HostConfig.PortBindings != nil {
		for portBinding, bindings := range container.HostConfig.PortBindings {
			if len(bindings) > 0 {
				for _, binding := range bindings {
					// 检查是否已经在 mappings 中
					found := false
					for i, m := range mappings {
						if m.ContainerPort == portBinding.Port() && m.Protocol == portBinding.Proto() {
							// 更新映射信息
							mappings[i].HostPort = binding.HostPort
							found = true
							break
						}
					}

					if !found {
						mapping := PortMapping{
							ContainerPort: portBinding.Port(),
							HostPort:      binding.HostPort,
							Protocol:      portBinding.Proto(),
						}
						mappings = append(mappings, mapping)
					}
				}
			}
		}
	}

	// 方法3：如果 HostPort 为空，尝试从实际监听的端口获取
	for i := range mappings {
		if mappings[i].HostPort == "" {
			// 尝试查找实际分配的主机端口
			mappings[i].HostPort = findActualHostPort(container, mappings[i].ContainerPort, mappings[i].Protocol)
		}
	}

	return mappings
}

func findActualHostPort(container types.ContainerJSON, containerPort, protocol string) string {
	// 尝试从 NetworkSettings.Ports 查找实际的端口绑定
	if container.NetworkSettings != nil {
		for portSpec, bindings := range container.NetworkSettings.Ports {
			if portSpec.Port() == containerPort && portSpec.Proto() == protocol {
				if len(bindings) > 0 {
					for _, binding := range bindings {
						if binding.HostPort != "" {
							return binding.HostPort
						}
					}
				}
			}
		}
	}

	// 如果没有找到，返回空字符串
	return ""
}

// parsePortAndProtocol 解析端口和协议
func parsePortAndProtocol(portProto string) (string, string) {
	// 格式如 "80/tcp"
	for i := 0; i < len(portProto); i++ {
		if portProto[i] == '/' {
			return portProto[:i], portProto[i+1:]
		}
	}
	return portProto, "tcp" // 默认协议
}
