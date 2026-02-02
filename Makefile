.PHONY: all frontend backend clean run stop

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

run: stop backend
	@echo "Running backend in background..."
	nohup ./bin/uea > uea.log 2>&1 &
	@echo "Backend started. Check uea.log for output."
	@echo "Access the frontend at http://localhost:8080"

stop:
	@echo "Stopping any running backend instances..."
	-pkill -f "bin/uea" || true