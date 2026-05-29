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
		return nil, fmt.Errorf("dockroot not available")
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
	if err := s.client.Pull(req.Image, req.Name); err != nil {
		return fmt.Errorf("pull image: %w", err)
	}
	if req.Command != "" {
		return fmt.Errorf("custom command not supported in dockroot mode")
	}
	return s.client.Run(req.Name, req.Detach, req.Env, req.Volumes)
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
	return s.client.Run(name, true, nil, nil)
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
	return s.client.Pull(image, name)
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
