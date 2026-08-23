# Deploy, Local Development & Testing

> Three flows in one doc: production deploy with Docker Compose (directly or
> via a PaaS), local development with `make dev` or `docker compose`, and the
> testing matrix.

---

## Production deploy

`docker-compose.yml` works with plain `docker compose up -d` and with any
platform that consumes a Compose file (Dokploy, Coolify, CapRover, Portainer
stacks, …). Two values in it are **literal and meant to be edited**: the
published host port and the host config directory. Everything else comes from
the deployment environment.

> **Do not turn those two into shell-style variables.** Some platforms reject
> variable substitution inside a volume source and refuse to deploy, and worse,
> a config directory that resolves to the wrong path fails *silently*: the
> entrypoint sees an empty `/app/config`, seeds the embedded skeleton, and the
> stock dashboard comes up looking healthy while your YAML is nowhere in sight.

### Initial setup

1. Push the repo to a git remote (private is fine) — or clone it on the host.
2. If you use a PaaS, create an app of type "Docker Compose" pointing at the
   repo. It reads `docker-compose.yml` and builds the image from the
   `Dockerfile` at the repo root.
3. Create the host config directory and point the compose file at it —
   edit the bind mount under `volumes:` and the host side of `ports:`:

   ```bash
   sudo mkdir -p /opt/myserver/config
   sudo chown $(id -u):$(id -g) /opt/myserver/config
   ```

4. Copy the skeletons and edit them:

   ```bash
   cp internal/config/skeleton/*.yaml /opt/myserver/config/
   $EDITOR /opt/myserver/config/{services,settings,widgets,bookmarks,docker}.yaml
   ```

5. Configure the environment (a `.env` next to the compose file, or your
   platform's environment UI):

   ```
   TZ=Etc/UTC
   HOMEPAGE_ALLOWED_HOSTS=dashboard.example.com,dashboard.example.com:443
   HOMEPAGE_SCRIPTS_ENABLED=true                   # optional

   # Only if you enable the built-in email allowlist (step 7)
   HOMEPAGE_VAR_GOOGLE_CLIENT_ID=…apps.googleusercontent.com
   HOMEPAGE_VAR_GOOGLE_CLIENT_SECRET=GOCSPX-…
   ```

   `HOMEPAGE_ALLOWED_HOSTS` is not wildcarded by default: if you leave it
   unset, only localhost is accepted and your public hostname returns 400.

   Compose does not forward a host variable to the container unless the
   service names it under `environment:`. `docker-compose.yml` already
   declares the two Google variables with no value for exactly this reason;
   the same applies on a PaaS, where its environment UI populates the
   deployment and the Compose file passes it through.

6. Point your reverse proxy / tunnel at the published port.

   To serve the dashboard under a path instead of at the root of the host, set
   `HOMEPAGE_BASE_PATH=/team` (see
   [`configuration.md`](./configuration.md#serving-under-a-base-path)) and
   configure the proxy to pass the prefix through **unstripped** — the instance
   expects `/team/…` and answers `404` outside it. Remember to prefix the
   healthcheck and the Google `redirectURL` too.

7. **Decide who can see the dashboard.** It is public by default, so it needs
   one of:

   - an auth layer in front (Cloudflare Access, Authelia, oauth2-proxy,
     Tailscale…), or
   - the built-in email allowlist: drop a `config/auth.yaml` listing the
     addresses allowed in, and Google sign-in becomes mandatory. Full setup in
     [`authentication.md`](./authentication.md).

   > **Set the environment variables before writing `auth.yaml`.** A file whose
   > `{{HOMEPAGE_VAR_*}}` placeholders cannot be resolved makes the process
   > refuse to start, so the wrong order gives you a restart loop. Verify with
   > `docker exec <container> printenv | grep HOMEPAGE_VAR_GOOGLE`.
   >
   > To go back to public later, empty the allowlist (`emails: []`) — do not
   > delete the file, which answers 503 on everything by design.

> **You never need to shell into the container.** The config directory is a
> host bind mount; edit on the host and `fsnotify` reloads inside the
> container automatically.

### Volumes

| Host | Container | Purpose |
|---|---|---|
| the host dir in `volumes:` | `/app/config` | User YAML + scripts + data (bind mount, hot-reload). |
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

Orchestrators use this for unhealthy-container detection and zero-downtime
restarts. `/api/healthcheck` is the one endpoint that must stay reachable
without auth.

With `HOMEPAGE_BASE_PATH` set, the path moves with the rest of the app:
`http://localhost:3000/team/api/healthcheck`. An unprefixed healthcheck answers
`404` and reports the container unhealthy for the wrong reason.

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
