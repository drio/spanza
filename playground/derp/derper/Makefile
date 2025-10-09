# Makefile for DERP server playground

TAILSCALE_DIR = /home/drio/dev/github.com/tailscale/tailscale
DERPER_SRC = $(TAILSCALE_DIR)/cmd/derper
DERPER_BIN = ./derper

.PHONY: build run-dev run-prod clean help test-connection gen-prod-key install-systemd uninstall-systemd

help: ## Show this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-10s %s\n", $$1, $$2}'

build: ## Build the derper binary from Tailscale source
	@echo "Building derper from $(DERPER_SRC)..."
	cd $(DERPER_SRC) && go build -o $(CURDIR)/derper
	@echo "Built: $(DERPER_BIN)"

run-dev: build ## Build and run derper in dev mode (localhost:3340)
	@echo "Starting DERP server in dev mode..."
	@echo "Server will run on http://localhost:3340"
	@echo "Press Ctrl+C to stop"
	@echo ""
	$(DERPER_BIN) --dev

clean: ## Remove built binary
	rm -f $(DERPER_BIN)
	@echo "Cleaned up derper binary"

test-connection: ## Test if dev server is running
	@echo "Testing connection to DERP server..."
	@curl -s http://localhost:3340 > /dev/null && \
		echo "✓ Server is running!" || \
		echo "✗ Server is not responding"

gen-prod-key: build ## Generate persistent key for production
	@echo "Generating production key..."
	@mkdir -p ./prod/config
	@if [ -f ./prod/config/derper.key ]; then \
		echo "Key already exists at ./prod/config/derper.key"; \
		echo "Delete it first if you want to generate a new one"; \
	else \
		$(DERPER_BIN) -c ./prod/config/derper.key --hostname=localhost & \
		DERPER_PID=$$! && \
		sleep 2 && \
		kill $$DERPER_PID 2>/dev/null || true && \
		echo "✓ Production key generated at ./prod/config/derper.key"; \
	fi

run-prod: build ## Run in production mode (requires HOSTNAME variable)
	@if [ -z "$(HOSTNAME)" ]; then \
		echo "Error: HOSTNAME is required"; \
		echo "Usage: make run-prod HOSTNAME=derp.example.com"; \
		exit 1; \
	fi
	@if [ ! -f ./prod/config/derper.key ]; then \
		echo "No production key found. Run 'make gen-prod-key' first"; \
		exit 1; \
	fi
	@mkdir -p ./prod/certs
	@echo "Starting DERP server in production mode..."
	@echo "Hostname: $(HOSTNAME)"
	@echo "Port: 443 (requires sudo or CAP_NET_BIND_SERVICE)"
	@echo "Certificate mode: LetsEncrypt"
	@echo ""
	@echo "NOTE: This will fail without:"
	@echo "  - Domain $(HOSTNAME) pointing to this server"
	@echo "  - Ports 80 and 443 publicly accessible"
	@echo "  - Root permissions or CAP_NET_BIND_SERVICE capability"
	@echo ""
	sudo $(DERPER_BIN) \
		-c ./prod/config/derper.key \
		--hostname=$(HOSTNAME) \
		--certmode=letsencrypt \
		--certdir=./prod/certs \
		-a :443

install-systemd: build ## Install as systemd service (requires HOSTNAME and sudo)
	@if [ -z "$(HOSTNAME)" ]; then \
		echo "Error: HOSTNAME is required"; \
		echo "Usage: sudo make install-systemd HOSTNAME=derp.example.com"; \
		exit 1; \
	fi
	@echo "Installing DERP server as systemd service..."
	sudo useradd -r -s /bin/false derper 2>/dev/null || true
	sudo mkdir -p /var/lib/derper/certs
	sudo cp $(DERPER_BIN) /usr/local/bin/derper
	sudo chown root:root /usr/local/bin/derper
	@if [ -f ./prod/config/derper.key ]; then \
		sudo cp ./prod/config/derper.key /var/lib/derper/derper.key; \
		echo "Copied existing key to /var/lib/derper/derper.key"; \
	else \
		echo "Generating new key at /var/lib/derper/derper.key"; \
		sudo /usr/local/bin/derper -c /var/lib/derper/derper.key --hostname=localhost & \
		DERPER_PID=$$! && \
		sleep 2 && \
		sudo kill $$DERPER_PID 2>/dev/null || true; \
	fi
	sudo chown derper:derper /var/lib/derper/derper.key
	sudo chmod 600 /var/lib/derper/derper.key
	sudo chown derper:derper /var/lib/derper/certs
	@echo "Creating systemd service file..."
	@echo '[Unit]' | sudo tee /etc/systemd/system/derper.service > /dev/null
	@echo 'Description=Tailscale DERP Server' | sudo tee -a /etc/systemd/system/derper.service > /dev/null
	@echo 'After=network.target' | sudo tee -a /etc/systemd/system/derper.service > /dev/null
	@echo '' | sudo tee -a /etc/systemd/system/derper.service > /dev/null
	@echo '[Service]' | sudo tee -a /etc/systemd/system/derper.service > /dev/null
	@echo 'Type=simple' | sudo tee -a /etc/systemd/system/derper.service > /dev/null
	@echo 'User=derper' | sudo tee -a /etc/systemd/system/derper.service > /dev/null
	@echo 'ExecStart=/usr/local/bin/derper -c /var/lib/derper/derper.key --hostname=$(HOSTNAME) --certmode=letsencrypt --certdir=/var/lib/derper/certs -a :443' | sudo tee -a /etc/systemd/system/derper.service > /dev/null
	@echo 'Restart=always' | sudo tee -a /etc/systemd/system/derper.service > /dev/null
	@echo 'RestartSec=5' | sudo tee -a /etc/systemd/system/derper.service > /dev/null
	@echo 'AmbientCapabilities=CAP_NET_BIND_SERVICE' | sudo tee -a /etc/systemd/system/derper.service > /dev/null
	@echo '' | sudo tee -a /etc/systemd/system/derper.service > /dev/null
	@echo '[Install]' | sudo tee -a /etc/systemd/system/derper.service > /dev/null
	@echo 'WantedBy=multi-user.target' | sudo tee -a /etc/systemd/system/derper.service > /dev/null
	sudo systemctl daemon-reload
	@echo "✓ Installed systemd service"
	@echo ""
	@echo "To start the service:"
	@echo "  sudo systemctl enable derper"
	@echo "  sudo systemctl start derper"
	@echo "  sudo systemctl status derper"
	@echo ""
	@echo "To view logs:"
	@echo "  sudo journalctl -u derper -f"

uninstall-systemd: ## Uninstall systemd service (requires sudo)
	@echo "Uninstalling DERP systemd service..."
	-sudo systemctl stop derper 2>/dev/null
	-sudo systemctl disable derper 2>/dev/null
	sudo rm -f /etc/systemd/system/derper.service
	sudo rm -f /usr/local/bin/derper
	sudo systemctl daemon-reload
	@echo "✓ Service removed"
	@echo ""
	@echo "NOTE: /var/lib/derper still exists (contains keys/certs)"
	@echo "To remove: sudo rm -rf /var/lib/derper"
	@echo "To remove user: sudo userdel derper"
