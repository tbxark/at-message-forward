BUILD_DIR=./build
MODULE := $(shell go list -m)
BUILD=$(shell git rev-parse --short HEAD)@$(shell date +%s)
CURRENT_OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
CURRENT_ARCH := $(shell uname -m | tr '[:upper:]' '[:lower:]')
LD_FLAGS=-ldflags "-X main.BuildVersion=$(BUILD)"
GO_BUILD=CGO_ENABLED=0 go build $(LD_FLAGS)

.PHONY: build
build:
	$(GO_BUILD) -o $(BUILD_DIR)/ ./...

.PHONY: buildLinuxX86
buildLinuxX86:
	GOOS=linux GOARCH=amd64 $(GO_BUILD) -o $(BUILD_DIR)/ ./...

# buildUSB builds with the libusb-backed USB transport (`transport: "usb"`).
# Requires libusb (macOS: `brew install libusb`; Debian/Ubuntu: `apt install libusb-1.0-0-dev`).
.PHONY: buildUSB
buildUSB:
	CGO_ENABLED=1 go build -tags usb $(LD_FLAGS) -o $(BUILD_DIR)/ ./cmd/atmsgfwd


.PHONY: buildImage
buildImage:
	docker buildx build --platform=linux/amd64,linux/arm64 -t ghcr.io/tbxark/at-message-forward:latest . --push --provenance=false

.PHONY: lint
lint:
	go fmt ./...
	go vet ./...
	go get ./...
	go test ./...
	go mod tidy
	golangci-lint fmt --no-config --enable gofmt,goimports
	golangci-lint run --no-config --fix
	nilaway -include-pkgs="$(MODULE)" ./...
