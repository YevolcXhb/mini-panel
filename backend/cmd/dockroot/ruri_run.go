package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/spf13/cobra"
)

type ruriRunOptions struct {
	global   *globalOptions
	renew    bool
	hostname string
	workDir  string
	network  string
	restart  string
	envVars  []string
	volumes  []string
	publish  []string
	detach   bool
}

func ruriRunCmd(global *globalOptions) *cobra.Command {
	opts := ruriRunOptions{global: global}
	cmd := &cobra.Command{
		Use:     "run NAME [COMMAND [ARGS]]",
		Short:   "run image from a rootfs",
		RunE:    commandAction(opts.run),
		Example: `DockRoot run alpine001 [COMMAND [ARGS]]`,
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.renew, "renew", false, "Renew config")
	flags.BoolVarP(&opts.detach, "detach", "d", false, "Run container in detached mode")
	flags.StringVar(&opts.hostname, "hostname", "", "Hostname inside the container")
	flags.StringVarP(&opts.workDir, "workdir", "w", "", "Working directory inside the container")
	flags.StringVar(&opts.network, "network", "", "Network inside the container, support host only")
	flags.StringVar(&opts.restart, "restart", "", "Restart policy, not support")
	flags.StringSliceVarP(&opts.envVars, "env", "e", []string{}, "Set environment variables (e.g., -e UID=0 -e GID=0)")
	flags.StringSliceVarP(&opts.volumes, "volume", "v", []string{}, "Bind mount a volume (e.g., -v /mnt:/mnt)")
	flags.StringSliceVarP(&opts.publish, "publish", "p", []string{}, "Publish a container's port(s) to the host. not support")
	return cmd
}

func (opts *ruriRunOptions) run(args []string, stdout io.Writer) (retErr error) {
	if len(args) < 1 {
		return fmt.Errorf("Usage: %s run NAME", os.Args[0])
	}
	if !opts.renew {
		if opts.hostname != "" ||
			opts.workDir != "" ||
			len(opts.envVars) > 0 ||
			len(opts.volumes) > 0 {
			return fmt.Errorf("Cannot specify options without --renew")
		}
	}
	if opts.network != "" && opts.network != "host" {
		return fmt.Errorf("Invalid network option, only 'host' is supported")
	}
	if len(opts.publish) > 0 {
		return fmt.Errorf("Publishing ports is not supported")
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
		opts.renew = true
	}
	if opts.renew {
		err = writeRuri(ruriPath,
			destAbsDir, opts.hostname,
			opts.workDir,
			opts.envVars,
			opts.volumes)
		if err != nil {
			return err
		}
	}

	env := os.Environ()
	argExtras := args[1:]
	var argsToRun []string
	if opts.detach {
		logFile := filepath.Join(destAbsDir, "ruri.log")
		if len(argExtras) == 0 {
			argsToRun = []string{
				"-b", "-L", logFile, "-c", confPath,
			}
		} else {
			argsToRun = make([]string, 0, len(argExtras)+6)
			argsToRun = append(argsToRun, []string{
				"-b", "-L", logFile, "-c", confPath,
			}...)
			argsToRun = append(argsToRun, argExtras...)
		}
		cmd := exec.Command(ruriPath, argsToRun...)
		cmd.Env = append(os.Environ(), env...)
		// 最小权限设置
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true, // 仅设置进程组
			//Setsid:  true,
		}
		// 保留标准流但重定向到/dev/null
		if devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0); err == nil {
			defer devNull.Close()
			cmd.Stdin = devNull
			cmd.Stdout = devNull
			cmd.Stderr = devNull
		}
		err = cmd.Start()
		if err != nil {
			return err
		}
		err = cmd.Process.Release()
		if err != nil {
			return err
		}
		cmd.Wait()
	} else {
		if len(argExtras) == 0 {
			argsToRun = []string{
				filepath.Base(ruriPath),
				"-c", confPath,
			}
		} else {
			argsToRun = make([]string, 0, len(argExtras)+6)
			argsToRun = append(argsToRun, []string{
				filepath.Base(ruriPath),
				"-c", confPath,
			}...)
			argsToRun = append(argsToRun, argExtras...)
		}
		err = syscall.Exec(ruriPath, argsToRun, env)
		if err != nil {
			return err
		}
	}
	return nil
}

func writeRuri(ruriPath,
	destAbsDir,
	hostname,
	workDir string,
	envs,
	volumes []string) error {
	var targetBashStr string
	if _, err := os.Stat(filepath.Join(destAbsDir, "rootfs", "bin", "bash")); err == nil {
		targetBashStr = "/bin/bash"
	} else {
		targetBashStr = "/bin/sh"
	}

	txts := []string{
		"nameserver 223.5.5.5",
		"",
	}
	err := os.WriteFile(filepath.Join(destAbsDir, "/rootfs/etc/resolv.conf"),
		[]byte(strings.Join(txts, "\n")), 0644)
	if err != nil {
		return err
	}

	spec, err := getSpecConfig(filepath.Join(destAbsDir, "config.json"))
	if err != nil {
		return err
	}
	if hostname != "" && spec.Hostname != hostname {
		spec.Hostname = hostname
		saveSpecConfig(filepath.Join(destAbsDir, "config.json"), spec)
	}

	ruriInfo := DefaultRuriInfo()
	ruriInfo.RuriPath = ruriPath
	ruriInfo.ContainerDir = filepath.Join(destAbsDir, "rootfs")

	if len(hostname) > 0 {
		ruriInfo.Hostname = hostname
	} else {
		ruriInfo.Hostname = CleanString(filepath.Base(destAbsDir))
	}

	if len(workDir) > 0 {
		ruriInfo.WorkDir = workDir
	} else {
		ruriInfo.WorkDir = spec.Process.Cwd
	}

	envMap := make(map[string]struct{})
	if len(envs) > 0 {
		for _, env := range envs {
			ss := strings.SplitN(env, "=", 2)
			if len(ss) == 2 && len(ss[1]) > 0 {
				ruriInfo.Envs = append(ruriInfo.Envs, ss[0], ss[1])
				envMap[ss[0]] = struct{}{}
			}
		}
	}
	if len(spec.Process.Env) > 0 {
		for _, env := range spec.Process.Env {
			ss := strings.SplitN(env, "=", 2)
			if len(ss) == 2 && len(ss[1]) > 0 {
				if _, ok := envMap[ss[0]]; !ok {
					ruriInfo.Envs = append(ruriInfo.Envs, ss[0], ss[1])
				}
			}
		}
	}

	if len(spec.Process.Args) > 0 {
		if spec.Process.Args[0] == "/init" {
			if isHomeassistant(spec) {
				entry, err := writeHomeassistant(targetBashStr, destAbsDir)
				if err == nil {
					ruriInfo.Commands = append(ruriInfo.Commands, entry)
				}
			}
		} else {
			ruriInfo.Commands = spec.Process.Args
		}
	}
	if len(ruriInfo.Commands) == 0 {
		ruriInfo.Commands = append(ruriInfo.Commands, targetBashStr)
	}

	if len(volumes) > 0 {
		for _, vol := range volumes {
			ss := strings.SplitN(vol, ":", 3)
			if len(ss) >= 2 && len(ss[1]) > 0 {
				if len(ss) == 3 && ss[2] == "ro" {
					ruriInfo.ExtraRoMountpoints = append(ruriInfo.ExtraRoMountpoints, ss[0], ss[1])
				} else {
					ruriInfo.ExtraMountpoints = append(ruriInfo.ExtraMountpoints, ss[0], ss[1])
				}
			}
		}
	}

	ruriConf, err := os.OpenFile(filepath.Join(destAbsDir, "ruri.conf"),
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer ruriConf.Close()
	return RenderRuriInfo(ruriInfo, ruriConf)
}

func isHomeassistant(spec *rspec.Spec) bool {
	if strings.Contains(spec.Hostname, "home-assistant") {
		return true
	}
	for _, env := range spec.Process.Env {
		if strings.Contains(env, "home-assistant.io") {
			return true
		}
	}
	return false
}

func writeHomeassistant(targetBashStr, destAbsDir string) (string, error) {
	f, err := ruriFiles.Open("ruri/homeassistant.sh")
	if err != nil {
		return "", err
	}
	defer f.Close()
	targetPath := "root/entry.sh"
	targetAbsPath := filepath.Join(destAbsDir, "rootfs", targetPath)
	fOut, err := os.OpenFile(targetAbsPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return "", err
	}
	defer fOut.Close()
	_, err = fmt.Fprintf(fOut, "#!%s\n", targetBashStr)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(fOut, f)
	if err != nil {
		return "", err
	}
	return "/" + targetPath, nil
}
