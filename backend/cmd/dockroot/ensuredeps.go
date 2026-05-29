package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

type ensureDepsOptions struct {
	global *globalOptions
}

func ensureDepsCmd(global *globalOptions) *cobra.Command {
	opts := ensureDepsOptions{global: global}
	cmd := &cobra.Command{
		Use:     "ensuredeps",
		Short:   "download dependencies",
		RunE:    commandAction(opts.run),
		Example: `DockRoot ensuredeps`,
	}
	return cmd
}

type registryInfo struct {
	Mirrors     []string `json:"registry-mirrors"`
	DataRoot    string   `json:"data-root"`
	UseKspeeder bool     `json:"useKspeeder"`
}

func readRegistryInfo(binaryDir string) (*registryInfo, error) {
	data, err := os.ReadFile(filepath.Join(binaryDir, "dockroot.json"))
	if err != nil {
		return nil, err
	}
	var info registryInfo
	err = json.Unmarshal(data, &info)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func writeDefaultRegistry(binaryDir string) error {
	var err error
	baseDir := filepath.Base(binaryDir)
	var dataRoot string
	if baseDir == "DockRootBin" {
		dataRoot = filepath.Join(filepath.Dir(binaryDir), "DockRootData")
	} else {
		dataRoot = filepath.Join(binaryDir, "DockRootData")
	}
	info := &registryInfo{
		Mirrors: []string{
			"https://registry.istoreos.com",
			"https://docker1.linkease.com:60005",
			"https://kooldocker.openpop.cn",
			"https://kooldocker.gvpu.cn",
			"https://docker.1ms.run",
			"https://docker.m.daocloud.io",
		},
		UseKspeeder: true,
		DataRoot:    dataRoot,
	}
	if _, err = os.Stat(info.DataRoot); os.IsNotExist(err) {
		err = os.MkdirAll(info.DataRoot, 0755)
		if err != nil {
			return err
		}
	}
	return writeRegistryInfo(binaryDir, info)
}

func writeRegistryInfo(binaryDir string, info *registryInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(binaryDir, "dockroot.json"), data, 0644)
}

func (opts *ensureDepsOptions) run(args []string, stdout io.Writer) (retErr error) {
	binaryDir, err := getBinaryDir()
	if err != nil {
		return err
	}
	info, err := readRegistryInfo(binaryDir)
	if err == nil {
		_, err = os.Stat(info.DataRoot)
	}
	if err != nil {
		err = writeDefaultRegistry(binaryDir)
		if err != nil {
			return err
		}
	}

	client := &http.Client{}
	kspeederBin := filepath.Join(binaryDir, "kspeeder")
	if !checkIsKspeederDownload(kspeederBin) {
		if err := downloadKspeeder(client, kspeederBin); err != nil {
			return err
		}
	}

	ruriPath := filepath.Join(binaryDir, "ruri")
	if !checkIsRuriDownload(ruriPath) {
		if err := downloadBinary(client,
			RuriUrl,
			ruriPath,
			"ruri"); err != nil {
			return err
		}
		if !checkIsBinaryDownload(ruriPath, "-v", "ruri version") {
			return fmt.Errorf("failed to download ruri binary")
		}
	}
	return nil
}
