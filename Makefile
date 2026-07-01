.PHONY: dev stop logs install

# Start everything: backend (detached) + UI dev server
dev: install
	@echo "Starting backend services..."
	@$(MAKE) -C backend docker-up
	@echo "Starting UI on http://localhost:5173 ..."
	@cd ui && npm run dev

# Install UI dependencies (safe to run multiple times)
install:
	@cd ui && npm install --silent

# Stop backend docker services
stop:
	@$(MAKE) -C backend docker-down

# Tail backend API logs
logs:
	@$(MAKE) -C backend docker-logs
