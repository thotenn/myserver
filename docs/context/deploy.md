# Deploy, Local Development & Testing

> Three flows in one doc: production deploy with Coolify, local development
> with `make dev` or `docker compose`, and the testing matrix.

---

## Deploy with Coolify

### Initial setup

1. Push the repo to GitHub (private is fine).
2. Create a Private App in Coolify pointing to the repo via GitHub App.
3. Coolify detects `docker-compose.yml` and runs the build + deploy.
4. Create the host config directory:

   ```bash
   sudo mkdir -p /opt/myserver/config
   sudo chown $(id -u):$(id -g) /opt/myserver/config
   ```

5. Copy skeletons and edit:

   ```bash
   cp internal/config/skeleton/*.yaml /opt/myserver/config/
   $EDITOR /opt/myserver/config/{services,settings,widgets,bookmarks,docker}.yaml
   ```

6. Configure environment in Coolify:

   ```
   HOMEPAGE_ALLOWED_HOSTS=htop.thotenn.com,localhost:3000
   HOMEPAGE_SCRIPTS_ENABLED=true     # optional
   TZ=America/Asuncion
   ```

7. Cloudflare Tunnel: point `htop.thotenn.com` at port 3000 of the
   container.

> **You never need to shell into the container.** `config/` is a host
> bind mount; edit on the host and `fsnotify` reloads in the container
> automatically.

### Volumes

| Host | Container | Purpose |
|---|---|---|
| `/opt/myserver/config` | `/app/config` | User YAML + scripts + data (bind mount, hot-reload). |
| `/var/run/docker.sock` | `/var/run/docker.sock` | Docker stats + script wrappers. |

Mount the socket `:ro` when scripts don't need to mutate containers.

### Resource limits (`docker-compose.yml`)

```yaml
deploy:
  resources:
    limits:       { memory: 256M, cpus: "1.0" }
    reservations: { memory: 64M }
```

Real-world usage sits around 50–80 MiB; 256 MiB leaves plenty of headroom.

### Healthcheck

```yaml
healthcheck:
  test: ["CMD", "wget", "-qO-", "http://localhost:3000/api/healthcheck"]
  interval: 30s
  timeout:  5s
  retries:  3
  start_period: 10s
```

Coolify uses this for unhealthy-container detection and zero-downtime
restarts.

---

## Local development

### Requirements

- **Go 1.25+**
- **templ** CLI: `go install github.com/a-h/templ/cmd/templ@v0.3.1001`
  (pin to match `go.mod`)
- **Tailwind CSS v3 standalone CLI**:

  ```bash
  curl -sLo /usr/local/bin/tailwindcss \
    https://github.com/tailwindlabs/tailwindcss/releases/download/v3.4.17/tailwindcss-linux-x64
  chmod +x /usr/local/bin/tailwindcss
  ```

- **air** for hot reload: `go install github.com/air-verse/air@latest`

### First time

```bash
git clone <repo>
cd myserver
go mod download
make build
```

### Iteration

```bash
make dev                                # air: templ + tailwind + go + restart

# Or individually:
make templ        # regenerate _templ.go
make tailwind     # recompile CSS
go build -o myserver ./cmd/myserver/
./myserver -port 3030
```

### Local demo config

```bash
mkdir -p /tmp/myserver-demo-config
# Drop your services.yaml, settings.yaml, etc. into the directory
HOMEPAGE_CONFIG_DIR=/tmp/myserver-demo-config ./myserver -port 3030
```

Then open `http://localhost:3030/`.

To regenerate a comprehensive demo `config/` exercising every feature:

```bash
./bootstrap-demo-config.sh
```

### Docker Compose (dev)

`docker-compose.dev.yml` bind-mounts `./config/` from the host so you can
edit YAMLs in any editor and the container hot-reloads automatically.

```bash
mkdir -p config
cp internal/config/skeleton/*.yaml config/
$EDITOR config/services.yaml

make up      # docker compose up -d
make logs    # tail
make down    # stop
```

`./config/` is gitignored so each developer keeps their own setup.

#### Caveats on the dev compose

- **`userns_mode: keep-id`** maps container uid 1000 to the host user that
  started Podman (no-op under Docker Engine). Without it the entrypoint's
  `chown myserver:myserver /app/config` leaves the bind-mounted directory
  unwritable to the host user. If you already hit it:
  `podman unshare chown -R 0:0 ./config`.
- **`:Z`** on the `./config:/app/config:Z` bind mount tells Podman to
  apply a private SELinux relabel — required on Fedora / RHEL / CentOS;
  no-op elsewhere.
- The Docker / Podman socket mount is commented out by default. Uncomment
  one line (`/var/run/docker.sock` or
  `/run/user/1000/podman/podman.sock`) when you need container stats or
  scripts that talk to the daemon.
- The default published port is **8085** (avoids clashing with the
  common 3000); `HOMEPAGE_ALLOWED_HOSTS` lists both.

---

## Testing

### Current coverage

| Package | Coverage |
|---|---|
| `internal/scripts` | 66.7 % |
| `internal/discovery` | 43.4 % |
| `internal/config` | 43.4 % |
| `internal/middleware` | 38.9 % |
| `internal/proxy` | 29.7 % |
| `internal/widgets` | 23.1 % |
| `internal/handlers` | 10.2 % |

### Critical adversarial tests

- **`internal/scripts/manager_test.go`** — 14 tests:
  path traversal (`../../etc/passwd`), symlinks pointing outside the
  whitelist, absolute paths, non-`.sh` extensions, prefix collisions
  (`/app/scripts` vs `/app/scriptsbak`), env denylist (`LD_PRELOAD`,
  `PATH`, `BASH_ENV`, `IFS`), `allowConcurrent: false`, timeouts that
  kill the process tree, output cap with `yes` / `cat /dev/urandom`,
  race conditions in `StreamOutput` (`-race`), hot-reload via
  `ReplaceAll`.
- **`internal/middleware/host_validation_test.go`** — 5 tests:
  defaults always include localhost; explicit `*` wildcard; list + port
  handling; case-insensitive match; rejection logging.
- **`internal/handlers/handlers_test.go`** — 5 tests:
  `/api/services` strips basic-auth and widget credentials;
  `/api/widgets` sanitizes recursively (nested `clientSecret`, …);
  `/api/scripts` → 404 when disabled; `/api/hash` reflects
  `config.CurrentHash()`; `/api/config` honors the whitelist.

### Run tests

```bash
make test          # all packages
make test-race     # with race detector — mandatory before merging changes in internal/scripts/
make test-cover    # coverage per package
```

---

## Make targets

| Target | Action |
|---|---|
| `make build` | `templ generate` + `tailwindcss --minify` + `go build` |
| `make test` | `go test ./...` |
| `make test-race` | `go test -race ./...` |
| `make test-cover` | per-package coverage |
| `make lint` | `gofmt -l` + `go vet` |
| `make templ` | regenerate `*_templ.go` |
| `make tailwind` | compile `web/static/css/main.css` |
| `make dev` | hot reload with air |
| `make tidy` | `go mod tidy` |
| `make clean` | remove binary, generated CSS, `*_templ.go` |
| `make up` / `make down` / `make logs` | docker compose wrappers |
| `make docker-build` | build image |
| `make docker-run` | run with local volumes |
| `make generate` | alias of `templ` + `tailwind` |
