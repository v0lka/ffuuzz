.PHONY: build-frontend build-backend build dev-frontend dev-backend clean lint test

build-frontend:
	cd web && npm ci && npm run build

build-backend:
	go build -o ffuuzz ./cmd/ffuuzz

build: build-frontend build-backend

dev-frontend:
	cd web && npm run dev

dev-backend:
	go run ./cmd/ffuuzz serve

clean:
	rm -rf web/dist/* ffuuzz
	@touch web/dist/.gitkeep

lint:
	cd web && npm run lint
	golangci-lint run

test:
	go test ./... -race
