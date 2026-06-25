package service

import (
	"fmt"
	"strings"

	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/utils/dockroot"
)

type ContainerService struct {
	client *dockroot.Client
}

func NewContainerService() *ContainerService {
	return &ContainerService{client: global.DockRootClient}
}

func (s *ContainerService) List() ([]dto.ContainerListResponse, error) {
	if s.client == nil {
		return []dto.ContainerListResponse{}, nil
	}
	states, err := s.client.ListAll()
	if err != nil {
		return nil, err
	}
	var result []dto.ContainerListResponse
	for _, st := range states {
		result = append(result, dto.ContainerListResponse{
			Name:      st.Name,
			Image:     st.Image,
			Status:    st.Status,
			PIDs:      st.PIDs,
			CreatedAt: st.CreatedAt,
			Rootfs:    st.Rootfs,
		})
	}
	return result, nil
}

func (s *ContainerService) Inspect(name string) (*dto.ContainerListResponse, error) {
	if s.client == nil {
		return nil, fmt.Errorf("dockroot not available")
	}
	st, err := s.client.InspectContainer(name)
	if err != nil {
		return nil, err
	}
	return &dto.ContainerListResponse{
		Name:      st.Name,
		Image:     st.Image,
		Status:    st.Status,
		PIDs:      st.PIDs,
		CreatedAt: st.CreatedAt,
		Rootfs:    st.Rootfs,
	}, nil
}

func (s *ContainerService) Create(req dto.ContainerCreateRequest) error {
	if s.client == nil {
		return fmt.Errorf("dockroot not available")
	}
	// 自动命名：若用户未指定容器名，从镜像中提取
	if strings.TrimSpace(req.Name) == "" {
		req.Name = dockroot.ExtractContainerName(req.Image)
		if req.Name == "" {
			return fmt.Errorf("cannot extract container name from image %q", req.Image)
		}
	}
	if _, err := s.client.Pull(req.Image, req.Name); err != nil {
		return fmt.Errorf("pull image: %w", err)
	}
	if req.Command != "" {
		return fmt.Errorf("custom command not supported in dockroot mode")
	}
	_, err := s.client.Run(req.Name, req.Detach, req.Env, req.Volumes)
	return err
}

func (s *ContainerService) Start(name string) error {
	if s.client == nil {
		return fmt.Errorf("dockroot not available")
	}
	st, err := s.client.InspectContainer(name)
	if err != nil {
		return err
	}
	if st.Status == "running" {
		return fmt.Errorf("container already running")
	}
	_, err = s.client.Run(name, true, nil, nil)
	return err
}

func (s *ContainerService) Stop(name string) error {
	if s.client == nil {
		return fmt.Errorf("dockroot not available")
	}
	return s.client.Stop(name)
}

func (s *ContainerService) Remove(name string) error {
	if s.client == nil {
		return fmt.Errorf("dockroot not available")
	}
	return s.client.Rm(name)
}

func (s *ContainerService) Logs(name string, tail int) (string, error) {
	if s.client == nil {
		return "", fmt.Errorf("dockroot not available")
	}
	return s.client.ReadLog(name, tail)
}

func (s *ContainerService) ListFiles(name, path string) ([]string, error) {
	if s.client == nil {
		return nil, fmt.Errorf("dockroot not available")
	}
	infos, err := s.client.ListContainerFiles(name, path)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, info := range infos {
		names = append(names, info.Name())
	}
	return names, nil
}

func (s *ContainerService) Pull(image, name string) error {
	if s.client == nil {
		return fmt.Errorf("dockroot not available")
	}
	_, err := s.client.Pull(image, name)
	return err
}

func (s *ContainerService) IsAvailable() bool {
	return s.client != nil
}

func (s *ContainerService) GetContainerEnv(name string) ([]string, error) {
	dir := global.GetDataDir()
	confPath := fmt.Sprintf("%s/%s/ruri.conf", dir, name)
	data, err := global.DockRootClient.CopyFromContainer(name, "/etc/environment")
	if err != nil {
		return nil, err
	}
	var envs []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			envs = append(envs, line)
		}
	}
	_ = confPath
	return envs, nil
}
