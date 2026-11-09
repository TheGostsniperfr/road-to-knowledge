.PHONY: init check test fmt build dev serve validate mcp

DATA ?= $(RTK_DATA)
GOFILES = cmd internal web/embed.go

init: ## install Go and npm dependencies
	go mod download
	cd web && npm ci

check: ## everything CI runs
	@test -z "$$(gofmt -l $(GOFILES))" || (gofmt -l $(GOFILES); echo "run make fmt"; exit 1)
	go vet ./...
	go test ./...
	cd web && npx tsc --noEmit && npx vitest run

test:
	go test ./...
	cd web && npx vitest run

fmt:
	gofmt -w $(GOFILES)

build: ## web app embedded into bin/rtk
	cd web && npm run build
	go build -o bin/rtk ./cmd/rtk

serve: build ## make serve DATA=path/to/knowledge
	./bin/rtk serve --data $(DATA)

dev: ## Go API on :7878 and Vite with hot reload on :5173
	@test -n "$(DATA)" || (echo "set DATA or RTK_DATA"; exit 1)
	go run ./cmd/rtk serve --data $(DATA) & cd web && npm run dev; kill %1

validate:
	go run ./cmd/rtk validate --data $(DATA)

mcp:
	go run ./cmd/rtk mcp --data $(DATA)
