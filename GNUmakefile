TEST?=$$(go list ./... | grep -v /vendor/)
GOFMT_FILES?=$$(find . -name '*.go' | grep -v vendor)
PKG_NAME=shc

default: build

build:
	go build ./...

test:
	go test -v -cover -timeout=120s -parallel=10 ./...

testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./...

vet:
	go vet ./...

lint: vet
	golangci-lint run

fmt:
	gofmt -w $(GOFMT_FILES)

docs:
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest generate

docs-validate:
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest validate
