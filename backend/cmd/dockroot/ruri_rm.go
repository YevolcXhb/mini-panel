package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

type ruriRmOptions struct {
	global *globalOptions
	force  bool
}

func ruriRmCmd(global *globalOptions) *cobra.Command {
	opts := ruriRmOptions{global: global}
	cmd := &cobra.Command{
		Use:     "rm NAME",
		Short:   "umount image from a rootfs",
		RunE:    commandAction(opts.run),
		Example: `DockRoot rm alpine001`,
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.force, "force", false, "force umount rootfs")
	return cmd
}

func (opts *ruriRmOptions) run(args []string, stdout io.Writer) (retErr error) {
	if len(args) != 1 {
		return fmt.Errorf("Usage: %s run NAME", os.Args[0])
	}
	binaryDir, err := getBinaryDir()
	if err != nil {
		return err
	}
	info, err := readRegistryInfo(binaryDir)
	if err != nil {
		return err
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
	pids, err := RuriPids(ruriPath, confPath)
	if err != nil {
		return err
	}
	if len(pids) > 0 {
		if opts.force {
			KillProcess(pids)
		} else {
			return fmt.Errorf("ruri is running, use -f to force stop")
		}
	}
	return RunRuri(ruriPath, []string{"-U", confPath}, stdout)
}
