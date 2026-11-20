GO ?= go
VERSION ?= 1.4.2
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build run test fmt vet tidy linux release clean docker installer-check

build:
	mkdir -p bin
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/pulse ./cmd/pulse

run:
	$(GO) run ./cmd/pulse --addr 127.0.0.1:8080 --db ./data/pulse.db

test:
	$(GO) test ./...

fmt:
	gofmt -w cmd internal

vet:
	$(GO) vet ./...

tidy:
	$(GO) mod tidy

linux:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/pulse-linux-amd64 ./cmd/pulse
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/pulse-linux-arm64 ./cmd/pulse

release: linux
	cd dist && sha256sum pulse-linux-* > SHA256SUMS

clean:
	rm -rf bin dist

docker:
	docker compose up -d --build

installer-check:
	bash -n install.sh
	bash -n deploy/install-agent.sh
