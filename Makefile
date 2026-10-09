# agents-tree: common tasks. `make` or `make help` lists them.
BIN      ?= bin/agents-tree
SESSION  ?=
PERMS    ?= all
SPEED    ?= 4
CMD      := ./cmd/agents-tree

.DEFAULT_GOAL := help
.PHONY: help build run continue pick replay sessions context baseline hooks-status test vet fmt check install clean

help: ## list the targets and their variables
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  make %-14s %s\n", $$1, $$2}'
	@echo
	@echo "variables: SESSION=path/to/session.jsonl  PERMS=all|accept-edits|plan  SPEED=4  BIN=$(BIN)"
	@echo "examples:  make run   |  make run PERMS=plan   |  make replay SESSION=~/.claude/projects/x/y.jsonl"

build: ## build the binary into bin/agents-tree
	@mkdir -p $(dir $(BIN))
	go build -o $(BIN) $(CMD)

run: build ## follow the latest session of the current directory (SESSION=... to pick one, PERMS=... for Claude's rights)
	$(BIN) live $(SESSION) --permissions $(PERMS)

pick: build ## open the app with the session picker (new session or an existing one)
	$(BIN) live --pick --permissions $(PERMS)

replay: build ## replay a session: make replay SESSION=file.jsonl [SPEED=4]
	@test -n "$(SESSION)" || { echo "usage: make replay SESSION=path/to/session.jsonl"; exit 2; }
	$(BIN) replay $(SESSION) --speed $(SPEED)

sessions: build ## list sessions, newest first
	$(BIN) sessions

context: build ## where the input tokens (and money) of a session go (SESSION=... optional)
	$(BIN) context $(SESSION)

baseline: build ## what the first request contains and what could be cut (SESSION=... optional)
	$(BIN) baseline $(SESSION)

hooks-status: build ## show which hooks are installed (never changes anything)
	$(BIN) hooks status

test: ## run all tests
	go test ./... -count=1

vet: ## go vet
	go vet ./...

fmt: ## format the code
	gofmt -w cmd internal

check: ## format check + vet + tests (run before committing)
	@test -z "$$(gofmt -l cmd internal)" || { echo "not formatted:"; gofmt -l cmd internal; exit 1; }
	go vet ./...
	go test ./... -count=1

install: ## install agents-tree into GOBIN (default ~/go/bin)
	go install $(CMD)

clean: ## remove build output
	rm -rf bin
