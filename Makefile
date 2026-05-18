APP_NAME    := kish
BUILD_DIR   := build
DOCKER_REPO := ghcr.io/maple52046
MAIN        := cmd/kish/main.go
VERSION     := $(shell git describe --tags --always)

GOOS        ?= linux
GOARCH      ?= amd64
CGO_ENABLED ?= 0

.PHONY: build build-docker clean test

build:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags "-s -w -X main.Version=$(VERSION)" -o build/kish ./cmd/kish

build-docker:
	docker build --build-arg VERSION=$(VERSION) -t $(DOCKER_REPO)/$(APP_NAME):$(VERSION) .

clean:
	rm -rf $(BUILD_DIR)

test:
	go test ./...
