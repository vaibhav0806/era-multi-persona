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

.PHONY: abigen
abigen: ## Regenerate iNFT abigen bindings from contracts/out
	@command -v jq >/dev/null || { echo "ERROR: jq not installed (brew install jq)"; exit 1; }
	@command -v abigen >/dev/null || { echo "ERROR: abigen not installed (go install github.com/ethereum/go-ethereum/cmd/abigen@v1.17.2)"; exit 1; }
	cd contracts && forge build
	mkdir -p era-brain/inft/zg_7857/bindings
	jq '.abi' contracts/out/EraPersonaINFT.sol/EraPersonaINFT.json > /tmp/era_inft.abi
	abigen --abi /tmp/era_inft.abi --pkg bindings --type EraPersonaINFT \
	  --out era-brain/inft/zg_7857/bindings/era_persona_inft.go
	@echo "Bindings regenerated."

VPS_HOST ?= era@178.105.44.3

.PHONY: deploy
deploy: ## Rsync repo to VPS and restart service (runs go builds on VPS with native GOARCH)
	rsync -az --delete \
	    --exclude bin/ --exclude pi-agent.db --exclude pi-agent.db-wal --exclude pi-agent.db-shm \
	    --exclude node_modules/ --exclude .env --exclude .git \
	    ./ $(VPS_HOST):/opt/era/
	ssh $(VPS_HOST) 'export PATH=/usr/local/go/bin:$$PATH && cd /opt/era && go env GOARCH && make build && make docker-runner && sudo systemctl restart era && sudo systemctl status era'
