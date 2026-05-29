package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type ruriPidsOptions struct {
	global *globalOptions
	detail bool
}

func ruriPidsCmd(global *globalOptions) *cobra.Command {
	opts := ruriPidsOptions{global: global}
	cmd := &cobra.Command{
		Use:     "ps NAME",
		Short:   "get pids of running rootfs",
		RunE:    commandAction(opts.run),
		Example: `DockRoot ps alpine001`,
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.detail, "detail", false, "details of pids")
	return cmd
}

func (opts *ruriPidsOptions) run(args []string, stdout io.Writer) (retErr error) {
	binaryDir, err := getBinaryDir()
	if err != nil {
		return err
	}
	info, err := readRegistryInfo(binaryDir)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return listDockers(binaryDir, info, stdout)
	}

	hostname := CleanString(args[0])
	destDir := filepath.Join(info.DataRoot, hostname)
	destAbsDir, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}

	ruriPath := filepath.Join(binaryDir, "ruri")
	if !checkIsRuriDownload(ruriPath) {
		client := &http.Client{}
		if err := downloadBinary(client,
			"https://fw0.koolcenter.com/binary/DockRoot/ruri",
			ruriPath,
			"ruri"); err != nil {
			return err
		}
		if !checkIsBinaryDownload(ruriPath, "-v", "ruri version") {
			return fmt.Errorf("failed to download ruri binary")
		}
	}
	confPath := filepath.Join(destAbsDir, "ruri.conf")
	if _, err := os.Stat(confPath); err != nil {
		return err
	}
	if opts.detail {
		return RunRuri(ruriPath, []string{"-P", confPath}, stdout)
	}
	pids, err := RuriPids(ruriPath, confPath)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, strings.Join(pids, " "))
	return nil
}

func listDockers(binaryDir string, info *registryInfo, stdout io.Writer) (retErr error) {
	paths, err := os.ReadDir(info.DataRoot)
	if err != nil {
		return err
	}
	for _, p := range paths {
		if !p.IsDir() {
			continue
		}
		if isDirValid(filepath.Join(info.DataRoot, p.Name())) {
			fmt.Fprintln(stdout, p.Name())
		}
	}
	return nil
}

func isDirValid(dir string) bool {
	for _, p := range []string{"config.json", "rootfs"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			return false
		}
	}
	return true
}
