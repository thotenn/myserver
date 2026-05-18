.PHONY: build test test-race test-cover lint clean templ tailwind docker-build docker-run run dev generate tidy up down logs

BINARY=myserver
DOCKER_IMAGE=myserver
GO=go
TEMPL=$(HOME)/go/bin/templ

# Tailwind CSS standalone CLI (no Node.js). Install via:
#   curl -sLo /usr/local/bin/tailwindcss \
#     https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-x64
#   chmod +x /usr/local/bin/tailwindcss
TAILWIND ?= tailwindcss

build: templ tailwind
	$(GO) build -ldflags="-s -w" -o $(BINARY) ./cmd/myserver/

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

test-cover:
	$(GO) test -cover ./...
	$(GO) test -coverprofile=cover.out ./...
	$(GO) tool cover -func=cover.out

lint:
	gofmt -l . | tee /dev/stderr | (! read)
	$(GO) vet ./...

templ:
	$(TEMPL) generate

tailwind:
	$(TAILWIND) -i web/tailwind/input.css -o web/static/css/main.css -c tailwind.config.js --minify

dev: ## Hot reload with air
	air

clean:
	rm -f $(BINARY) cover.out
	rm -f web/static/css/main.css
	find . -name '*_templ.go' -delete

docker-build:
	docker build -t $(DOCKER_IMAGE) .

docker-run: docker-build
	docker run -d --name myserver \
		-p 3000:3000 \
		-v ./config:/app/config \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-e HOMEPAGE_CONFIG_DIR=/app/config \
		-e HOMEPAGE_ALLOWED_HOSTS='localhost:3000' \
		-e TZ=America/Asuncion \
		$(DOCKER_IMAGE)

tidy: ## Run go mod tidy
	$(GO) mod tidy

up: ## Start services with docker compose
	docker compose up -d

down: ## Stop services with docker compose
	docker compose down

logs: ## Tail docker compose logs
	docker compose logs -f

generate: templ tailwind ## Generate templ + tailwind (alias for build prep)
