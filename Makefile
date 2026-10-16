GOARCH_LINUX ?= $(shell go env GOARCH)

.PHONY: build build-linux contract-linux test lint reqcheck gate

build:
	go build -o bin/macka ./cmd/macka

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH_LINUX) go build -o bin/macka-linux-$(GOARCH_LINUX) ./cmd/macka

test:
	go test ./...

lint:
	@test -z "$$(gofmt -l . | tee /dev/stderr)" || (echo "gofmt needed" && exit 1)
	go vet ./...

reqcheck:
	./scripts/check-req-tests.sh

contract-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH_LINUX) go test -c -tags contract -o bin/contract-linux-$(GOARCH_LINUX).test ./test/contract

gate: build-linux contract-linux
	./test/gate/run-m0-gate.sh bin/macka-linux-$(GOARCH_LINUX) bin/contract-linux-$(GOARCH_LINUX).test
