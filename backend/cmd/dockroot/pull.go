package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/containers/common/pkg/retry"
	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type pullOptions struct {
	global              *globalOptions
	deprecatedTLSVerify *deprecatedTLSVerifyOption
	srcImage            *imageOptions
	destImage           *imageDestOptions
	retryOpts           *retry.Options
	digestFile          string // Write digest to this file
}

func pullCmd(global *globalOptions) *cobra.Command {
	sharedFlags, sharedOpts := sharedImageFlags()
	deprecatedTLSVerifyFlags, deprecatedTLSVerifyOpt := deprecatedTLSVerifyFlags()
	srcFlags, srcOpts := imageFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	destFlags, destOpts := imageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	retryFlags, retryOpts := retryFlags()
	opts := pullOptions{global: global,
		deprecatedTLSVerify: deprecatedTLSVerifyOpt,
		srcImage:            srcOpts,
		destImage:           destOpts,
		retryOpts:           retryOpts,
	}
	cmd := &cobra.Command{
		Use:     "pull IMAGE:TAG NAME",
		Short:   "pull an image from a registry",
		RunE:    commandAction(opts.run),
		Example: `DockRoot pull alpine:latest alpine001`,
	}
	flags := cmd.Flags()
	flags.AddFlagSet(&sharedFlags)
	flags.AddFlagSet(&deprecatedTLSVerifyFlags)
	flags.AddFlagSet(&srcFlags)
	flags.AddFlagSet(&destFlags)
	flags.AddFlagSet(&retryFlags)
	flags.StringVar(&opts.digestFile, "digestfile", "", "Write the digest of the pushed image to the specified file")

	return cmd
}

// splitImageRef 解析镜像引用，支持带端口的 registry
// 格式: [docker://][registry_host[:port]/][namespace/]name[:tag|@digest]
func splitImageRef(ref string) (name, tag string) {
	ref = strings.TrimPrefix(ref, "docker://")

	// 处理 digest
	if at := strings.LastIndex(ref, "@"); at != -1 {
		return ref[:at], ref[at+1:]
	}

	// 找最后一个 ':'，判断是否是 tag 分隔符（tag 中不含 '/'）
	if colon := strings.LastIndex(ref, ":"); colon != -1 {
		after := ref[colon+1:]
		if !strings.Contains(after, "/") && after != "" {
			return ref[:colon], after
		}
	}
	return ref, "latest"
}

func (opts *pullOptions) run(args []string, stdout io.Writer) (retErr error) {
	if len(args) != 2 {
		return fmt.Errorf("Usage: %s pull IMAGE:TAG DESTINATION", os.Args[0])
	}

	imageName, imageTag := splitImageRef(args[0])
	if imageName == "" {
		return fmt.Errorf("Invalid image format: %s", args[0])
	}

	binaryDir, err := getBinaryDir()
	if err != nil {
		return err
	}
	info, err := readRegistryInfo(binaryDir)
	if err != nil {
		err = writeDefaultRegistry(binaryDir)
		if err != nil {
			return err
		}
		info, err = readRegistryInfo(binaryDir)
		if err != nil {
			return err
		}
	} else {
		if _, err = os.Stat(info.DataRoot); err != nil {
			return err
		}
	}

	sourceURLs := buildPullSources(args[0], imageName, imageTag, info.Mirrors)
	if len(sourceURLs) == 0 {
		return fmt.Errorf("Invalid image format: %s", args[0])
	}

	var client *http.Client

	ruriPath := filepath.Join(binaryDir, "ruri")
	if !checkIsRuriDownload(ruriPath) {
		if client == nil {
			client = &http.Client{}
		}
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

	destDir := filepath.Join(info.DataRoot, CleanString(args[1]))
	if _, err := os.Stat(destDir); os.IsNotExist(err) {
		err = os.Mkdir(destDir, 0755)
		if err != nil {
			return err
		}
	} else {
		fmt.Printf("Destination directory %s already exists\n", destDir)
	}

	if !opts.global.debug {
		// Force debug logging if --debug is not set.
		logrus.SetLevel(logrus.DebugLevel)
	}

	if opts.global.policyPath == "" {
		opts.global.insecurePolicy = true
	}
	policyContext, err := opts.global.getPolicyContext()
	if err != nil {
		return fmt.Errorf("Error loading trust policy: %v", err)
	}
	defer func() {
		if err := policyContext.Destroy(); err != nil {
			retErr = noteCloseFailure(retErr, "tearing down policy context", err)
		}
	}()

	destName := fmt.Sprintf("oci:%s/images:%s", destDir, imageTag)
	destRef, err := alltransports.ParseImageName(destName)
	if err != nil {
		return fmt.Errorf("Invalid destination name %s: %v", destName, err)
	}

	sourceCtx, err := opts.srcImage.newSystemContext()
	if err != nil {
		return err
	}
	destinationCtx, err := opts.destImage.newSystemContext()
	if err != nil {
		return err
	}

	ctx, cancel := opts.global.commandTimeoutContext()
	defer cancel()

	opts.destImage.warnAboutIneffectiveOptions(destRef.Transport())

	var lastErr error
	for _, imageURL := range sourceURLs {
		if strings.Contains(imageURL, "registry.linkease.net:5443") {
			if client == nil {
				client = &http.Client{}
			}
			if err := checkAndRunKspeeder(
				filepath.Join(binaryDir, "kspeeder"),
				filepath.Join(info.DataRoot, "cache"),
				client,
			); err != nil {
				fmt.Printf("Skipping %s: %v\n", imageURL, err)
				lastErr = err
				continue
			}
		}

		srcRef, err := alltransports.ParseImageName(imageURL)
		if err != nil {
			fmt.Printf("Skipping %s: invalid source: %v\n", imageURL, err)
			lastErr = err
			continue
		}
		err = retry.IfNecessary(ctx, func() error {
			manifestBytes, err := copy.Image(ctx, policyContext, destRef, srcRef, &copy.Options{
				ReportWriter:         stdout,
				SourceCtx:            sourceCtx,
				DestinationCtx:       destinationCtx,
				MaxParallelDownloads: 2,
			})
			if err != nil {
				return err
			}
			if opts.digestFile != "" {
				manifestDigest, err := manifest.Digest(manifestBytes)
				if err != nil {
					return err
				}
				if err = os.WriteFile(opts.digestFile, []byte(manifestDigest.String()), 0644); err != nil {
					return fmt.Errorf("Failed to write digest to file %q: %w", opts.digestFile, err)
				}
			}
			return nil
		}, opts.retryOpts)
		if err != nil {
			fmt.Printf("Pull from %s failed: %v\n", imageURL, err)
			lastErr = err
			continue
		}

		err = unpack(fmt.Sprintf("%s/images", destDir), imageTag, destDir)
		os.RemoveAll(filepath.Join(destDir, "images"))
		destAbsDir, err2 := filepath.Abs(destDir)
		if err == nil && err2 == nil {
			imageName := imageURL
			ss := strings.Split(imageURL, "/")
			if len(ss) > 2 {
				imageName = strings.Join(ss[len(ss)-2:], "/")
			}
			err = writeRuri(ruriPath, destAbsDir, imageName, "", nil, nil)
		}
		return err
	}
	return fmt.Errorf("all image sources failed: %v", lastErr)
}

// explicitRegistry 判断镜像名是否显式携带仓库地址（主机名含 . 或端口）。
func explicitRegistry(ref string) bool {
	ref = strings.TrimPrefix(ref, "docker://")
	i := strings.Index(ref, "/")
	if i <= 0 {
		return false
	}
	first := ref[:i]
	return strings.Contains(first, ".") || strings.Contains(first, ":")
}

// mirrorHost 从镜像源配置（如 https://docker.1ms.run）中提取 registry 主机名。
func mirrorHost(mirror string) string {
	host := strings.TrimSpace(mirror)
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	return strings.Trim(host, "/")
}

// buildPullSources 生成候选的 docker:// 源地址。
// 显式带仓库地址的镜像只尝试自身；不带仓库地址的镜像依次尝试
// dockroot.json 中配置的镜像源，最后回退到 Docker Hub。
func buildPullSources(ref, name, tag string, mirrors []string) []string {
	if tag == "" {
		tag = "latest"
	}
	if explicitRegistry(ref) {
		return []string{fmt.Sprintf("docker://%s:%s", name, tag)}
	}
	var sources []string
	for _, m := range mirrors {
		host := mirrorHost(m)
		if host == "" {
			continue
		}
		sources = append(sources, fmt.Sprintf("docker://%s/%s:%s", host, name, tag))
	}
	sources = append(sources, fmt.Sprintf("docker://docker.io/%s:%s", name, tag))
	return sources
}

func CleanString(s string) string {
	// 匹配中文、英文、数字、空格
	reg := regexp.MustCompile(`[^\p{Han}a-zA-Z0-9-\s\\/]`)
	s2 := reg.ReplaceAllString(s, "")
	s2 = strings.Replace(s2, " ", "-", -1)
	s2 = strings.Replace(s2, "_", "-", -1)
	return strings.ToLower(s2)
}
