.PHONY: build test test-race test-cover lint clean templ tailwind docker-build docker-run run dev generate tidy up down logs dashboard

BINARY=myserver
DOCKER_IMAGE=myserver
GO=go
# Resolved from PATH; override if your templ lives elsewhere (TEMPL=/path/to/templ).
TEMPL ?= templ

# Tailwind CSS standalone CLI (no Node.js). v3 ONLY: v4 dropped @apply inside
# @layer base, which web/tailwind/input.css relies on. Install via (swap the
# asset for your platform: macos-arm64, macos-x64, linux-x64, linux-arm64):
#   curl -sLo ~/.local/bin/tailwindcss \
#     https://github.com/tailwindlabs/tailwindcss/releases/download/v3.4.17/tailwindcss-linux-x64
#   chmod +x ~/.local/bin/tailwindcss
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

# Local development. The binary defaults to the container path /app/config,
# which is not writable outside Docker, so point it at the repo's config/.
HOMEPAGE_CONFIG_DIR ?= ./config

dashboard: ## Scaffold a client dashboard config: SLUG=acme (see .agents/skills/sk-clients/)
	@test -n "$(SLUG)" || { echo "dashboard: pass SLUG=<slug>, e.g. make dashboard SLUG=acme" >&2; exit 1; }
	@bash .agents/skills/sk-clients/scripts/new-dashboard.sh $(SLUG)

dev: ## Hot reload with air
	HOMEPAGE_CONFIG_DIR=$(HOMEPAGE_CONFIG_DIR) air

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
		-e TZ=Etc/UTC \
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
