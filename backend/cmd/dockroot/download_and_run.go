package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	RuriUrl     = "https://fw0.koolcenter.com/binary/DockRoot/ruri"
	KspeederUrl = "https://fw0.koolcenter.com/binary/kspeeder/kspeeder-linux"
)

func checkIsBinaryDownload(binaryPath string, versionCmd, matchStr string) bool {
	cmd := exec.Command(binaryPath, versionCmd)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	if strings.Contains(string(b), matchStr) {
		return true
	}
	return false
}

func checkIsKspeederDownload(binaryPath string) bool {
	return checkIsBinaryDownload(binaryPath, "--help", "localAddr")
}

func downloadKspeeder(client *http.Client, binaryPath string) error {
	return downloadBinary(client,
		KspeederUrl,
		binaryPath,
		"kspeeder")
}

func downloadBinary(client *http.Client, urlPrefix, binaryPath, msgName string) error {
	fmt.Printf("Downloading %s... please wait. This may take a while.\n", msgName)
	ctx, cancelFn := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancelFn()
	req, err := http.NewRequestWithContext(ctx,
		http.MethodGet,
		fmt.Sprintf("%s.%s", urlPrefix, runtime.GOARCH),
		nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.OpenFile(binaryPath+".syn", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer func() {
		if f != nil {
			f.Close()
		}
	}()
	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return err
	}
	f.Close()
	f = nil
	if err := os.Rename(binaryPath+".syn", binaryPath); err != nil {
		return err
	}
	return nil
}

func runKspeeder(binaryPath, cachePath string, client *http.Client) error {
	fmt.Println("Running kspeeder... please wait. This may take a while.")
	cmd := exec.Command(binaryPath, "--cachePath", cachePath, "--exitAfter", "1")
	err := cmd.Start()
	if err != nil {
		return err
	}
	var ok bool
	for retry := 0; retry < 5; retry++ {
		time.Sleep(time.Second * 2)
		if checkIsKspeederRun(client) {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("Kspeeder did not start successfully")
	}
	fmt.Println("kspeeder started but wait for 10 seconds.")
	time.Sleep(time.Second * 10)
	fmt.Println("kspeeder started successfully. It will stop after 1 hours.")

	return nil
}

func checkAndRunKspeeder(binaryPath, cachePath string, client *http.Client) error {
	fmt.Println("checking kspeeder")
	if !checkIsKspeederRun(client) {
		if !checkIsKspeederDownload(binaryPath) {
			if err := downloadKspeeder(client, binaryPath); err != nil {
				return err
			}
		}
		return runKspeeder(binaryPath, cachePath, client)
	}
	return nil
}

func checkIsKspeederRun(client *http.Client) bool {
	ctx, cancelFn := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelFn()
	req, err := http.NewRequestWithContext(ctx,
		http.MethodGet,
		"https://registry.linkease.net:5443/v2/", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return false
	}
	if strings.HasPrefix(string(b), "{}") {
		return true
	}
	return false
}

func checkIsRuriDownload(binaryPath string) bool {
	return checkIsBinaryDownload(binaryPath, "-v", "ruri version")
}

func checkAndDownloadRuri(binaryPath string, client *http.Client) error {
	if !checkIsBinaryDownload(binaryPath, "-v", "ruri version") {
		if err := downloadBinary(client,
			RuriUrl,
			binaryPath,
			"ruri"); err != nil {
			return err
		}
		if !checkIsBinaryDownload(binaryPath, "-v", "ruri version") {
			return fmt.Errorf("failed to download ruri binary")
		}
	}
	return nil
}
