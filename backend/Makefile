GOCMD=go
GOBUILD=$(GOCMD) build
GOARCH=$(shell go env GOARCH)
GOOS=$(shell go env GOOS)

BASE_PATH := $(shell pwd)
BUILD_PATH = $(BASE_PATH)/../build
DIST_PATH = $(BASE_PATH)/../dist
MAIN=$(BASE_PATH)/cmd/server/main.go
NAME=minipanel
VERSION=1.0.0
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')

LDFLAGS=-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT) -s -w
TAGS=osusergo netgo static_build

.PHONY: all build build-android build-linux-amd64 build-linux-arm64 build-all clean dev test fmt vet lint release

all: build

build:
	@echo "Building for $(GOOS)/$(GOARCH)..."
	@cd $(BASE_PATH) && CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOBUILD) -trimpath -ldflags '$(LDFLAGS)' \
		-tags '$(TAGS)' -o $(BUILD_PATH)/$(NAME) $(MAIN)
	@echo "-> $(BUILD_PATH)/$(NAME)"

build-android:
	@echo "Building for linux/arm64 (Android)..."
	@cd $(BASE_PATH) && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) -trimpath -ldflags '$(LDFLAGS)' \
		-tags '$(TAGS)' -o $(BUILD_PATH)/$(NAME)-android-arm64 $(MAIN)
	@echo "-> $(BUILD_PATH)/$(NAME)-android-arm64"

build-linux-amd64:
	@echo "Building for linux/amd64..."
	@cd $(BASE_PATH) && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -trimpath -ldflags '$(LDFLAGS)' \
		-tags '$(TAGS)' -o $(BUILD_PATH)/$(NAME)-linux-amd64 $(MAIN)
	@echo "-> $(BUILD_PATH)/$(NAME)-linux-amd64"

build-linux-arm64:
	@echo "Building for linux/arm64..."
	@cd $(BASE_PATH) && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) -trimpath -ldflags '$(LDFLAGS)' \
		-tags '$(TAGS)' -o $(BUILD_PATH)/$(NAME)-linux-arm64 $(MAIN)
	@echo "-> $(BUILD_PATH)/$(NAME)-linux-arm64"

build-all: build-linux-amd64 build-linux-arm64 build-android
	@echo "All builds complete in $(BUILD_PATH)/"

clean:
	@rm -rf $(BUILD_PATH) $(DIST_PATH)
	@echo "Cleaned build artifacts"

dev:
	@cd $(BASE_PATH) && go run $(MAIN)

test:
	@cd $(BASE_PATH) && go test -v ./...

fmt:
	@cd $(BASE_PATH) && go fmt ./...

vet:
	@cd $(BASE_PATH) && go vet ./...

lint: fmt vet

release: build-all
	@mkdir -p $(DIST_PATH)
	@for binary in $(BUILD_PATH)/$(NAME)-*; do \
		if [ -f "$$binary" ]; then \
			filename=$$(basename "$$binary"); \
			platform=$${filename#$(NAME)-}; \
			tmpdir=$$(mktemp -d); \
			mkdir -p "$$tmpdir/minipanel"; \
			cp "$$binary" "$$tmpdir/minipanel/minipanel"; \
			cp -r $(BASE_PATH)/static "$$tmpdir/minipanel/" 2>/dev/null || true; \
			cp $(BASE_PATH)/../install.sh "$$tmpdir/minipanel/" 2>/dev/null || true; \
			cp $(BASE_PATH)/../README.md "$$tmpdir/minipanel/" 2>/dev/null || true; \
			cp $(BASE_PATH)/config.yaml.example "$$tmpdir/minipanel/" 2>/dev/null || true; \
			tar -czf "$(DIST_PATH)/minipanel-$(VERSION)-$$platform.tar.gz" -C "$$tmpdir" minipanel; \
			rm -rf "$$tmpdir"; \
			echo "Packaged: $(DIST_PATH)/minipanel-$(VERSION)-$$platform.tar.gz"; \
		fi \
	done
	@echo "Release packages created in $(DIST_PATH)/"
