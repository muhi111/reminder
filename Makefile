# Build the application
all: build

build:
	@echo "Building..."
	@go build -o main cmd/api/main.go

# Run the application
run:
	@if ! command -v dotenvx > /dev/null; then \
		echo "dotenvx is not installed. Please install it to use this feature."; \
		exit 1; \
	fi
	@dotenvx run -- go run cmd/api/main.go

# Clean up the project
clean:
	@echo "Cleaning up the project..."
	@rm -f main
	@rm -f *.db

# Live Reload
watch:
	@if ! command -v dotenvx > /dev/null; then \
		echo "dotenvx is not installed. Please install it to use this feature."; \
		exit 1; \
	fi
	$(eval DOMAIN := $(shell dotenvx get DOMAIN 2>/dev/null || echo ""))
	$(eval PORT := $(shell dotenvx get PORT 2>/dev/null || echo 8080))
	@if [ -z "$(DOMAIN)" ]; then \
		echo "Error: DOMAIN is not set in .env file"; \
		echo "Please generate a fixed domain from here: https://dashboard.ngrok.com/domains"; \
		echo "And add DOMAIN=your-domain.example.com to your .env file"; \
		exit 1; \
	fi
	@if command -v air > /dev/null; then \
		echo "Starting air for live reload..."; \
		if command -v ngrok > /dev/null; then \
			echo "Starting ngrok tunnel..."; \
			ngrok http --url=$(DOMAIN) $(PORT) > /dev/null & \
			ngrok_pid=$$!; \
			sleep 2; \
			if ! ps -p "$$ngrok_pid" > /dev/null; then \
				echo "Error: ngrok failed to start."; \
				exit 1; \
			fi; \
			echo "Ngrok is running in the background (PID: $$ngrok_pid)..."; \
			echo "You can access your app at: https://$(DOMAIN)"; \
		else \
			read -p "ngrok is not installed. Do you want to install it? [Y/n] " choice; \
			if [ "$$choice" != "n" ] && [ "$$choice" != "N" ]; then \
				echo "Please visit https://ngrok.com/download to install ngrok"; \
				echo "Continuing without ngrok..."; \
			fi; \
		fi; \
		dotenvx run -- air; \
	else \
		read -p "Go's 'air' is not installed on your machine. Do you want to install it? [Y/n] " choice; \
		if [ "$$choice" != "n" ] && [ "$$choice" != "N" ]; then \
			go install github.com/air-verse/air@latest; \
			dotenvx run -- air; \
			echo "Watching..."; \
		else \
			echo "You chose not to install air. Exiting..."; \
			exit 1; \
		fi; \
	fi

.PHONY: all build run clean watch
