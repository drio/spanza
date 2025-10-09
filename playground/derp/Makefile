# Makefile for DERP Playground

TAILSCALE_DIR = /home/drio/dev/github.com/tailscale/tailscale
GO = go

.PHONY: help init build-all build-keygen build-clientA build-clientB clean run-clientA run-clientB run-clientA-ts run-clientB-ts show-keys

help: ## Show this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

init: ## Initialize go module with dependencies
	@echo "Initializing module..."
	@if [ ! -f go.mod ]; then \
		$(GO) mod init github.com/drio/spanza/playground/derp; \
	fi
	@echo "Setting up Tailscale dependency..."
	$(GO) mod edit -replace tailscale.com=$(TAILSCALE_DIR)
	$(GO) mod tidy
	@echo "✓ Module initialized"

build-keygen: init ## Build keygen tool
	@echo "Building keygen..."
	$(GO) build -o bin/keygen ./cmd/keygen
	@echo "✓ Built: bin/keygen"

build-clientA: init ## Build clientA
	@echo "Building clientA..."
	$(GO) build -o bin/clientA ./cmd/clientA
	@echo "✓ Built: bin/clientA"

build-clientB: init ## Build clientB
	@echo "Building clientB..."
	$(GO) build -o bin/clientB ./cmd/clientB
	@echo "✓ Built: bin/clientB"

build-all: build-keygen build-clientA build-clientB ## Build all binaries

clean: ## Remove built binaries and go module files
	rm -rf bin/ go.mod go.sum
	@echo "✓ Cleaned up"

show-keys: build-keygen ## Show public keys for clients A and B
	@echo "Loading keys from bin/.env..."
	@. ./bin/.env && \
	echo "Client A:" && \
	echo "  Private: $$a" && \
	echo "  Public:  $$(echo "{\"privateKey\": \"$$a\"}" | ./bin/keygen --public)" && \
	echo "" && \
	echo "Client B:" && \
	echo "  Private: $$b" && \
	echo "  Public:  $$(echo "{\"privateKey\": \"$$b\"}" | ./bin/keygen --public)"

run-clientB: build-clientB ## Run client B (receiver/echo mode)
	@echo "Starting Client B (receiver)..."
	@. ./bin/.env && \
	./bin/clientB --server http://localhost:3340/derp --key $$b --echo

run-clientA: build-clientA ## Run client A (sender) - requires peer key
	@echo "Starting Client A (sender)..."
	@. ./bin/.env && \
	PEER_KEY=$$(echo "{\"privateKey\": \"$$b\"}" | ./bin/keygen --public) && \
	echo "Will send to peer: $$PEER_KEY" && \
	./bin/clientA --server http://localhost:3340/derp --key $$a --peer $$PEER_KEY

run-clientB-ts: build-clientB ## Run client B using Tailscale's public DERP server
	@echo "Starting Client B (receiver) using Tailscale DERP..."
	@echo "Server: https://derp.tailscale.com/derp (New York)"
	@echo ""
	@. ./bin/.env && \
	./bin/clientB --server https://derp.tailscale.com/derp --key $$b --echo

run-clientA-ts: build-clientA ## Run client A using Tailscale's public DERP server
	@echo "Starting Client A (sender) using Tailscale DERP..."
	@echo "Server: https://derp.tailscale.com/derp (New York)"
	@echo ""
	@. ./bin/.env && \
	PEER_KEY=$$(echo "{\"privateKey\": \"$$b\"}" | ./bin/keygen --public) && \
	echo "Will send to peer: $$PEER_KEY" && \
	./bin/clientA --server https://derp.tailscale.com/derp --key $$a --peer $$PEER_KEY
