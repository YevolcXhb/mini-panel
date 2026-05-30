package dockroot

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Client struct {
	BinaryPath string
	DataRoot   string
}

type ContainerState struct {
	Name      string   `json:"name"`
	Image     string   `json:"image"`
	Status    string   `json:"status"`
	PIDs      []string `json:"pids"`
	CreatedAt int64    `json:"created_at"`
	Rootfs    string   `json:"rootfs"`
}

type RegistryInfo struct {
	Mirrors     []string `json:"registry-mirrors"`
	DataRoot    string   `json:"data-root"`
	UseKspeeder bool     `json:"useKspeeder"`
}

func NewClient() (*Client, error) {
	return NewClientWithPath("")
}

func NewClientWithPath(binaryPath string) (*Client, error) {
	if binaryPath == "" {
		if p, err := exec.LookPath("dockroot"); err == nil {
			binaryPath = p
		} else if p, err := exec.LookPath("DockRoot"); err == nil {
			binaryPath = p
		} else {
			return nil, fmt.Errorf("dockroot binary not found in PATH")
		}
	}

	binaryDir := filepath.Dir(binaryPath)
	info, err := readRegistryInfo(binaryDir)
	if err != nil {
		return nil, fmt.Errorf("read dockroot registry info: %w", err)
	}

	return &Client{
		BinaryPath: binaryPath,
		DataRoot:   info.DataRoot,
	}, nil
}

func readRegistryInfo(binaryDir string) (*RegistryInfo, error) {
	data, err := os.ReadFile(filepath.Join(binaryDir, "dockroot.json"))
	if err != nil {
		return nil, err
	}
	var info RegistryInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *Client) Pull(image, name string) error {
	image = NormalizeImageRef(image)
	if image == "" {
		return fmt.Errorf("invalid image reference")
	}
	cmd := exec.Command(c.BinaryPath, "pull", image, name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("dockroot pull: %w, output: %s", err, string(output))
	}
	return nil
}

func (c *Client) Run(name string, detach bool, envs, volumes, ports []string) error {
	args := []string{"run"}
	if detach {
		args = append(args, "-d")
	}
	if len(volumes) > 0 || len(envs) > 0 || len(ports) > 0 {
		args = append(args, "--renew")
	}
	for _, p := range ports {
		args = append(args, "-p", p)
	}
	for _, v := range volumes {
		args = append(args, "-v", v)
	}
	for _, e := range envs {
		args = append(args, "-e", e)
	}
	args = append(args, name)
	cmd := exec.Command(c.BinaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("dockroot run: %w, output: %s", err, string(output))
	}
	return nil
}

func (c *Client) Stop(name string) error {
	return exec.Command(c.BinaryPath, "stop", name).Run()
}

func (c *Client) Rm(name string) error {
	if err := exec.Command(c.BinaryPath, "rm", name).Run(); err != nil {
		return err
	}
	dir := filepath.Join(c.DataRoot, name)
	return os.RemoveAll(dir)
}

func (c *Client) Ps(name string) ([]string, error) {
	out, err := exec.Command(c.BinaryPath, "ps", name).Output()
	if err != nil {
		return nil, err
	}
	pids := strings.Fields(string(out))
	return pids, nil
}

func (c *Client) ListContainers() ([]string, error) {
	out, err := exec.Command(c.BinaryPath, "ps").Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var names []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

func (c *Client) InspectContainer(name string) (*ContainerState, error) {
	dir := filepath.Join(c.DataRoot, name)
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("container %s not found", name)
	}

	pids, _ := c.Ps(name)
	status := "stopped"
	if len(pids) > 0 {
		status = "running"
	}

	return &ContainerState{
		Name:      name,
		Status:    status,
		PIDs:      pids,
		CreatedAt: info.ModTime().Unix(),
		Rootfs:    filepath.Join(dir, "rootfs"),
	}, nil
}

func (c *Client) ListAll() ([]ContainerState, error) {
	names, err := c.ListContainers()
	if err != nil {
		return nil, err
	}
	var states []ContainerState
	for _, name := range names {
		st, err := c.InspectContainer(name)
		if err != nil {
			continue
		}
		states = append(states, *st)
	}
	return states, nil
}

func (c *Client) ReadLog(name string, tail int) (string, error) {
	logPath := filepath.Join(c.DataRoot, name, "ruri.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if tail > 0 && len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	return strings.Join(lines, "\n"), nil
}

func (c *Client) CopyToContainer(name, containerPath string, data []byte) error {
	rootfs := filepath.Join(c.DataRoot, name, "rootfs")
	target := filepath.Join(rootfs, containerPath)
	if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(rootfs)) {
		return fmt.Errorf("path traversal detected")
	}
	return os.WriteFile(target, data, 0644)
}

func (c *Client) CopyFromContainer(name, containerPath string) ([]byte, error) {
	rootfs := filepath.Join(c.DataRoot, name, "rootfs")
	source := filepath.Join(rootfs, containerPath)
	if !strings.HasPrefix(filepath.Clean(source), filepath.Clean(rootfs)) {
		return nil, fmt.Errorf("path traversal detected")
	}
	return os.ReadFile(source)
}

func (c *Client) ListContainerFiles(name, path string) ([]os.FileInfo, error) {
	rootfs := filepath.Join(c.DataRoot, name, "rootfs")
	target := filepath.Join(rootfs, path)
	if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(rootfs)) {
		return nil, fmt.Errorf("path traversal detected")
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return nil, err
	}
	var infos []os.FileInfo
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func (c *Client) GetRegistryInfo() (*RegistryInfo, error) {
	binaryDir := filepath.Dir(c.BinaryPath)
	return readRegistryInfo(binaryDir)
}

func (c *Client) AddRegistryMirror(mirror string) error {
	binaryDir := filepath.Dir(c.BinaryPath)
	info, err := readRegistryInfo(binaryDir)
	if err != nil {
		info = &RegistryInfo{Mirrors: []string{}}
	}
	for _, m := range info.Mirrors {
		if m == mirror {
			return nil
		}
	}
	info.Mirrors = append(info.Mirrors, mirror)
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(binaryDir, "dockroot.json"), data, 0644)
}

func (c *Client) RemoveRegistryMirror(mirror string) error {
	binaryDir := filepath.Dir(c.BinaryPath)
	info, err := readRegistryInfo(binaryDir)
	if err != nil {
		return err
	}
	var newMirrors []string
	for _, m := range info.Mirrors {
		if m != mirror {
			newMirrors = append(newMirrors, m)
		}
	}
	info.Mirrors = newMirrors
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(binaryDir, "dockroot.json"), data, 0644)
}
