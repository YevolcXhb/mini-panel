package main

import (
	"encoding/json"
	"fmt"
	"os"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/umoci"
	"github.com/opencontainers/umoci/oci/cas/dir"
	"github.com/opencontainers/umoci/oci/casext"
	"github.com/opencontainers/umoci/oci/layer"
	"github.com/opencontainers/umoci/pkg/funchelpers"
)

func unpack(ociDir, imageTag, destDir string) (Err error) {
	engine, err := dir.Open(ociDir)
	if err != nil {
		return fmt.Errorf("open oci layout: %w", err)
	}
	engineExt := casext.NewEngine(engine)
	defer funchelpers.VerifyClose(&Err, engine)

	unpackOptions := layer.UnpackOptions{
		OnDiskFormat: layer.DirRootfs{},
	}
	if err := umoci.Unpack(engineExt, imageTag, destDir, unpackOptions); err != nil {
		return fmt.Errorf("unpack image: %w", err)
	}
	return nil
}

func getSpecConfig(path string) (*rspec.Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var spec rspec.Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

func saveSpecConfig(path string, spec *rspec.Spec) error {
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
