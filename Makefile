.PHONY: build web test docker run lint

web:
	cd web && npm install && npm run build

build: web
	go build -o bin/server ./cmd/server

test:
	go test ./... -v

docker:
	docker compose build

run:
	go run ./cmd/server

lint:
	go vet ./...
