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
	@sh -c "'$(GOFMT)' -l $$($(GOFMT_FILES))"

fmt:
	gofmt -w $(GOFMT_FILES)
