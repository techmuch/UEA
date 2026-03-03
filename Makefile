.PHONY: all frontend backend clean run stop start restart

all: frontend backend

frontend:
	@echo "Building frontend..."
	npm --prefix frontend install
	npm --prefix frontend run build -- --outDir ../web/static

backend:
	@echo "Building backend..."
	go build -o bin/uea ./cmd/uea

clean:
	@echo "Cleaning up build artifacts..."
	rm -rf bin
	rm -rf frontend/dist
	rm -rf web
	rm -f uea.log

# Default to background. Use --foreground for foreground.
# Example: make start --foreground
start: stop backend
ifneq (,$(filter --foreground,$(MAKECMDGOALS)))
	@echo "Running backend in foreground..."
	./bin/uea
else
	@echo "Running backend in background..."
	nohup ./bin/uea > uea.log 2>&1 &
	@echo "Backend started. Check uea.log for output."
	@echo "Access the frontend at http://localhost:8080"
endif

# Allow --foreground as a flag-like target
--foreground:
	@:

run: start

restart: stop start

stop:
	@echo "Stopping any running backend instances..."
	-pkill -f "bin/uea" || true