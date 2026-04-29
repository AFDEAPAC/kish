APP_NAME  := kish
BUILD_DIR := build
MAIN      := cmd/kish/main.go

GOOS        ?= linux
GOARCH      ?= amd64
CGO_ENABLED ?= 0

.PHONY: build clean test

build:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(BUILD_DIR)/$(APP_NAME) $(MAIN)

clean:
	rm -rf $(BUILD_DIR)

test:
	go test ./...
