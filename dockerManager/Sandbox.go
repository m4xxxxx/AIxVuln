package dockerManager

import (
	"AIxVuln/llm"
)

type Sandbox struct {
	ContainerId   string
	ContainerIp   string
	dm            *DockerManager
	SourceCodeDir string
	sandboxEnvMsg llm.EnvMessageX
}

func buildSandboxEnvMsg(containerID, containerIP string) llm.EnvMessageX {
	return llm.EnvMessageX{
		Key:       "AttackSandBoxInfo",
		Content:   map[string]interface{}{"ContainerId": containerID, "ContainerIP": containerIP},
		AppendEnv: false,
	}
}

func NewSandboxFromExisting(dm *DockerManager, containerID, containerIP, sourceCodeDir string) *Sandbox {
	s := &Sandbox{
		ContainerId:   containerID,
		ContainerIp:   containerIP,
		dm:            dm,
		SourceCodeDir: sourceCodeDir,
	}
	s.sandboxEnvMsg = buildSandboxEnvMsg(containerID, containerIP)
	return s
}

func NewSandbox(dm *DockerManager, sourceCodeDir string) (*Sandbox, error) {
	r, err := dm.Run("aisandbox", nil, 10, SetVolume(sourceCodeDir, "/sourceCodeDir"), SetWorkingDir("/sourceCodeDir"))
	if err != nil {
		return nil, err
	}
	s := &Sandbox{ContainerId: r.ContainerID, ContainerIp: r.IPAddress, dm: dm, SourceCodeDir: sourceCodeDir}
	s.sandboxEnvMsg = buildSandboxEnvMsg(s.ContainerId, s.ContainerIp)
	return s, nil
}

func (s *Sandbox) GetSandboxEnvMsg() *llm.EnvMessageX {
	return &s.sandboxEnvMsg
}

func (s *Sandbox) RunCommand(command []string, timeOutSecond int16) (string, error) {
	r, err := s.dm.DockerExec(s.ContainerId, command, timeOutSecond)
	if err != nil {
		return "", err
	}
	return r, nil
}
