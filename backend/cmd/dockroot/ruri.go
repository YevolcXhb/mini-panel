package main

import (
	"bufio"
	"embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"
)

//go:embed ruri/*
var ruriFiles embed.FS

type RuriInfo struct {
	RuriPath               string
	ContainerDir           string
	User                   string
	DropCaps               []string
	NoNewPrivs             bool
	EnableUnshare          bool
	Rootless               bool
	NoWarnings             bool
	CrossArch              string
	QemuPath               string
	UseRuriEnv             bool
	EnableSeccomp          bool
	HidePid                int
	CpuSet                 string
	CpuPercent             int
	Memory                 string
	JustChroot             bool
	UnmaskDirs             bool
	EnableMountHostRuntime bool
	WorkDir                string
	RootfsSource           string
	RoRoot                 bool
	NoNetwork              bool
	UseKvm                 bool
	OomScoreAdj            int
	ExtraMountpoints       []string
	ExtraRoMountpoints     []string
	Envs                   []string
	CharDevices            []string
	Commands               []string
	Hostname               string
	TimensMonotonicOffset  int
	TimensRealTimeOffset   int
	DenySyscalls           []string
}

func DefaultRuriInfo() *RuriInfo {
	return &RuriInfo{
		HidePid:    -114,
		NoWarnings: true,
		UseRuriEnv: true,
	}
}

func RenderRuriInfo(info *RuriInfo, w io.Writer) error {
	f, err := ruriFiles.Open("ruri/ruri.conf")
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	funcMap := template.FuncMap{
		"join2": func(elems []string) string {
			if len(elems) > 0 {
				return `"` + strings.Join(elems, `","`) + `"`
			}
			return ""
		},
	}
	templ := template.New("ruriconf").Funcs(funcMap)
	templ, err = templ.Parse(string(b))
	if err != nil {
		return err
	}

	return templ.Execute(w, info)
}

func RuriPids(ruriPath, ruriConf string) ([]string, error) {
	cmd := exec.Command(ruriPath, "-P", ruriConf)
	b, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	ss := strings.Split(string(b), "\n")
	var pids []string
	for _, s := range ss {
		if len(s) == 0 {
			continue
		}
		fields := strings.Fields(s)
		pids = append(pids, fields[0])
	}
	return pids, nil
}

func KillProcess(pids []string) error {
	var err2 error
	for _, pid := range pids {
		pidN, err := strconv.Atoi(pid)
		if err != nil {
			if err2 != nil {
				err2 = err
			}
			continue
		}
		process, err := os.FindProcess(pidN)
		if err != nil {
			if err2 != nil {
				err2 = err
			}
			continue
		}
		err = process.Kill()
		if err2 != nil {
			err2 = err
		}
	}
	return err2
}

func RunRuri(ruriPath string, args []string, stdout io.Writer) error {
	cmd := exec.Command(ruriPath, args...)
	outReader, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(outReader)
	for scanner.Scan() {
		fmt.Fprintln(stdout, scanner.Text())
	}

	return cmd.Wait()
}
