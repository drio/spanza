# Makefile for WireGuard userspace playground

.PHONY: all build clean server client test combined help fw-open fw-close fw-status

# Default relay port
RELAY_PORT ?= 51820

# Default target
all: build

# Build both server and client
build:
	@echo "Building WireGuard playground..."
	go build -o wg-server server.go
	go build -o wg-client client.go
	@echo "✓ Built wg-server and wg-client"

# Build combined binary
combined:
	@echo "Building combined WireGuard test..."
	go build -o wg-combined combined.go
	@echo "✓ Built wg-combined"
	@echo "Running combined test..."
	./wg-combined

# Run the server
server: build
	@echo "Starting WireGuard server (192.168.4.2:80)..."
	./wg-server

# Run the client (assumes server is already running)
client: build
	@echo "Starting WireGuard client (192.168.4.1)..."
	./wg-client

# Run full test: start server in background, run client, stop server
test: build
	@echo "Testing WireGuard connection..."
	@./wg-server & \
	SERVER_PID=$$! ; \
	sleep 2 ; \
	./wg-client ; \
	kill $$SERVER_PID 2>/dev/null || true

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f wg-server wg-client wg-combined wgplay
	@echo "✓ Cleaned"

# Firewall management for relay testing
fw-open:
	@echo "Opening UDP port $(RELAY_PORT) in firewall..."
	@if command -v ufw >/dev/null 2>&1; then \
		echo "Detected ufw (Ubuntu/Debian)..."; \
		sudo ufw allow $(RELAY_PORT)/udp; \
		sudo ufw status | grep $(RELAY_PORT); \
	elif command -v firewall-cmd >/dev/null 2>&1; then \
		echo "Detected firewalld (RHEL/CentOS/Fedora)..."; \
		sudo firewall-cmd --permanent --add-port=$(RELAY_PORT)/udp; \
		sudo firewall-cmd --reload; \
		sudo firewall-cmd --list-ports | grep $(RELAY_PORT); \
	elif command -v iptables >/dev/null 2>&1; then \
		echo "Using iptables..."; \
		sudo iptables -I INPUT -p udp --dport $(RELAY_PORT) -j ACCEPT; \
		sudo iptables -L INPUT -n | grep $(RELAY_PORT); \
	else \
		echo "❌ No supported firewall found (ufw, firewalld, or iptables)"; \
		exit 1; \
	fi
	@echo "✓ Port $(RELAY_PORT)/udp opened"

fw-close:
	@echo "Closing UDP port $(RELAY_PORT) in firewall..."
	@if command -v ufw >/dev/null 2>&1; then \
		echo "Detected ufw (Ubuntu/Debian)..."; \
		sudo ufw delete allow $(RELAY_PORT)/udp; \
	elif command -v firewall-cmd >/dev/null 2>&1; then \
		echo "Detected firewalld (RHEL/CentOS/Fedora)..."; \
		sudo firewall-cmd --permanent --remove-port=$(RELAY_PORT)/udp; \
		sudo firewall-cmd --reload; \
	elif command -v iptables >/dev/null 2>&1; then \
		echo "Using iptables..."; \
		sudo iptables -D INPUT -p udp --dport $(RELAY_PORT) -j ACCEPT 2>/dev/null || true; \
	else \
		echo "❌ No supported firewall found (ufw, firewalld, or iptables)"; \
		exit 1; \
	fi
	@echo "✓ Port $(RELAY_PORT)/udp closed"

fw-status:
	@echo "Checking firewall status for port $(RELAY_PORT)..."
	@if command -v ufw >/dev/null 2>&1; then \
		echo "=== UFW Status ==="; \
		sudo ufw status | grep -E "Status:|$(RELAY_PORT)" || echo "Port $(RELAY_PORT) not found in ufw rules"; \
	elif command -v firewall-cmd >/dev/null 2>&1; then \
		echo "=== Firewalld Status ==="; \
		sudo firewall-cmd --list-ports | grep $(RELAY_PORT) && echo "Port $(RELAY_PORT)/udp is open" || echo "Port $(RELAY_PORT) not found in firewalld rules"; \
	elif command -v iptables >/dev/null 2>&1; then \
		echo "=== iptables Status ==="; \
		sudo iptables -L INPUT -n | grep $(RELAY_PORT) || echo "Port $(RELAY_PORT) not found in iptables rules"; \
	else \
		echo "❌ No supported firewall found"; \
	fi

# Show help
help:
	@echo "WireGuard Playground Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  build     - Build server and client binaries"
	@echo "  server    - Run the WireGuard server (192.168.4.2:80)"
	@echo "  client    - Run the WireGuard client (192.168.4.1)"
	@echo "  test      - Run full test (server + client)"
	@echo "  combined  - Build and run combined single-binary test"
	@echo "  clean     - Remove built binaries"
	@echo "  fw-open   - Open UDP port in firewall (default: 51820)"
	@echo "  fw-close  - Close UDP port in firewall"
	@echo "  fw-status - Check firewall status for relay port"
	@echo "  help      - Show this help"
	@echo ""
	@echo "Firewall Usage:"
	@echo "  make fw-open               - Open port 51820/udp"
	@echo "  make fw-open RELAY_PORT=8080 - Open custom port"
	@echo "  make fw-status             - Check if port is open"
	@echo "  make fw-close              - Close the port"
	@echo ""
	@echo "Usage:"
	@echo "  Terminal 1: make server"
	@echo "  Terminal 2: make client"
	@echo ""
	@echo "  Or run: make test"
	@echo "  Or run: make combined"
