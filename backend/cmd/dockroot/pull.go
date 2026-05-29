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

func (opts *pullOptions) run(args []string, stdout io.Writer) (retErr error) {
	if len(args) != 2 {
		return fmt.Errorf("Usage: %s pull IMAGE:TAG DESTINATION", os.Args[0])
	}
	arg0Arr := strings.Split(args[0], ":")
	if len(arg0Arr) != 2 {
		return fmt.Errorf("Invalid image format: %s", args[0])
	}
	imageTag := arg0Arr[1]
	var imageURL string
	if strings.HasPrefix(arg0Arr[0], "docker://") {
		imageURL = arg0Arr[0]
	} else {
		i := strings.Index(arg0Arr[0], "/")
		if i > 0 && strings.Contains(arg0Arr[0], ".") {
			imageURL = "docker://" + arg0Arr[0]
		} else {
			imageURL = fmt.Sprintf("docker://registry.linkease.net:5443/%s", args[0])
		}
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

	var client *http.Client
	if strings.Contains(imageURL, "registry.linkease.net:5443") {
		client = &http.Client{}
		err = checkAndRunKspeeder(
			filepath.Join(binaryDir, "kspeeder"),
			filepath.Join(info.DataRoot, "cache"),
			client,
		)
		if err != nil {
			return err
		}
	}

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

	imageNames := []string{
		imageURL,
		fmt.Sprintf("oci:%s/images:%s", destDir, imageTag)}

	srcRef, err := alltransports.ParseImageName(imageNames[0])
	if err != nil {
		return fmt.Errorf("Invalid source name %s: %v", imageNames[0], err)
	}
	destRef, err := alltransports.ParseImageName(imageNames[1])
	if err != nil {
		return fmt.Errorf("Invalid destination name %s: %v", imageNames[1], err)
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
		return err
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

func CleanString(s string) string {
	// 匹配中文、英文、数字、空格
	reg := regexp.MustCompile(`[^\p{Han}a-zA-Z0-9-\s\\/]`)
	s2 := reg.ReplaceAllString(s, "")
	s2 = strings.Replace(s2, " ", "-", -1)
	s2 = strings.Replace(s2, "_", "-", -1)
	return strings.ToLower(s2)
}
