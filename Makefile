.PHONY: build test test-v lint fmt run clean smoke

BIN := bin/orchestrator

build:
	go build -o $(BIN) ./cmd/orchestrator

test:
	go test ./...

test-v:
	go test -v ./...

test-race:
	go test -race ./...

fmt:
	go fmt ./...
	goimports -w .

lint:
	go vet ./...

run: build
	./$(BIN)

clean:
	rm -rf bin/ *.db *.db-wal *.db-shm coverage.out

GOARCH ?= $(shell go env GOARCH)

BIN_RUNNER_LINUX := bin/era-runner-linux

runner-linux:
	GOOS=linux GOARCH=$(GOARCH) CGO_ENABLED=0 go build -o $(BIN_RUNNER_LINUX) ./cmd/runner

BIN_SIDECAR_LINUX := bin/era-sidecar-linux

sidecar-linux:
	GOOS=linux GOARCH=$(GOARCH) CGO_ENABLED=0 go build -o $(BIN_SIDECAR_LINUX) ./cmd/sidecar

docker-runner: runner-linux sidecar-linux
	docker build -t era-runner:m2 -f docker/runner/Dockerfile .

VPS_HOST ?= era@178.105.44.3

.PHONY: deploy
deploy: ## Rsync repo to VPS and restart service (runs go builds on VPS with native GOARCH)
	rsync -az --delete \
	    --exclude bin/ --exclude pi-agent.db --exclude pi-agent.db-wal --exclude pi-agent.db-shm \
	    --exclude node_modules/ --exclude .env --exclude .git \
	    ./ $(VPS_HOST):/opt/era/
	ssh $(VPS_HOST) 'cd /opt/era && /usr/local/go/bin/go env GOARCH && make build && make docker-runner && sudo systemctl restart era && sudo systemctl status era --no-pager'
