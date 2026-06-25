package cmd

import "os/exec"

func Which(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}
