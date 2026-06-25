VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BINARY  := terraform-provider-shc
OS      := $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH    := $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
PLUGIN_DIR := $$HOME/.terraform.d/plugins/registry.terraform.io/sovereignhybridcompute/shc/$(VERSION)/$(OS)_$(ARCH)

.PHONY: build install clean fmt vet tidy test

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) .

install: build
	@mkdir -p $(PLUGIN_DIR)
	cp $(BINARY) $(PLUGIN_DIR)/
	@echo "Installed $(BINARY) to $(PLUGIN_DIR)"

clean:
	rm -f $(BINARY)

fmt:
	gofmt -w .

vet:
	go vet ./...

tidy:
	go mod tidy

test:
	go test -v ./...
