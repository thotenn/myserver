# Running one instance per dashboard

Everything here is generic on purpose: hostnames, ports and paths are
placeholders (`example.com`, `<HOST_PORT>`, `/srv/myserver`). Substitute your
own — and keep the real values in your deployment environment, not in this repo.

---

## The environment of a dashboard instance

| Variable | Root dashboard | Client dashboard | Why |
|---|---|---|---|
| `HOMEPAGE_CONFIG_DIR` | `/app/config` | `/app/config/dashboards/acme` | One directory per dashboard, never shared. |
| `HOMEPAGE_BASE_PATH` | unset | `/acme` | Unset = the host root. |
| `HOMEPAGE_ALLOWED_HOSTS` | your hostname | the same hostname | `/api/*` refuses a `Host` it does not know. Both instances answer on one hostname, so both list it. |
| `HOMEPAGE_SCRIPTS_ENABLED` | your call | **never set** | Process-level. Unset ⇒ the endpoints are not registered at all. |
| `HOMEPAGE_ALLOW_PRIVATE_HOSTS` | as needed | `false` | A client dashboard has no reason to reach your internal network. |
| `HOMEPAGE_HSTS` | as needed | as needed | Only behind HTTPS. |
| `TRUSTED_PROXIES` | your proxy's CIDR | the same | Correct client IPs in the logs, and mandatory for the `trustedHeader` auth provider. |
| `TZ` | yours | the client's, if it differs | Affects `datetime` and script timestamps. |

A client instance needs **no** Docker socket and no `docker.yaml`.

## Compose skeleton

Two services, one image, one bind mount. The client instance mounts the same
config tree read-only and is pointed at a subdirectory of it:

```yaml
services:
  dashboard:
    image: myserver:latest
    environment:
      HOMEPAGE_ALLOWED_HOSTS: example.com
      TRUSTED_PROXIES: 10.0.0.0/8
    volumes:
      - /srv/myserver/config:/app/config
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:3000/api/healthcheck"]
      interval: 30s
      timeout: 5s
      retries: 3

  dashboard-acme:
    image: myserver:latest
    environment:
      HOMEPAGE_CONFIG_DIR: /app/config/dashboards/acme
      HOMEPAGE_BASE_PATH: /acme
      HOMEPAGE_ALLOWED_HOSTS: example.com
      HOMEPAGE_ALLOW_PRIVATE_HOSTS: "false"
      TRUSTED_PROXIES: 10.0.0.0/8
    volumes:
      - /srv/myserver/config:/app/config:ro
    healthcheck:
      # The prefix moves with everything else. The unprefixed path 404s, so an
      # unprefixed healthcheck reports the container unhealthy for the wrong
      # reason.
      test: ["CMD", "wget", "-qO-", "http://localhost:3000/acme/api/healthcheck"]
      interval: 30s
      timeout: 5s
      retries: 3
```

Both listen on 3000 inside their own container, so **no new host port is
needed** as long as the proxy reaches them over the container network. If your
proxy talks to published host ports instead, give the second instance another
one (`<HOST_PORT_2>:3000`) — internal, not exposed.

Mounting `:ro` on the client instance is worth the keystrokes: nothing in a
client dashboard is supposed to write.

## The reverse proxy

One rule per dashboard, and the prefix **must reach the instance intact**.

| Do | Do not |
|---|---|
| Route `/acme` and `/acme/*` to the client instance | Strip the prefix (`strip_prefix`, `rewrite ^/acme(.*) $1`, `PathPrefix` + `StripPrefix` middleware) |
| Route everything else to the root instance | Assume the root instance can answer `/acme` — it 404s, which is what you want |
| Match the longest prefix first (most proxies do; Traefik needs a higher `priority` on the more specific rule) | Leave the catch-all rule ahead of the specific one |

Nothing else changes: same hostname, so **no new DNS record, no new certificate,
and no new public port**. That is the whole reason to prefer a path over a
subdomain — a second-level subdomain (`acme.dash.example.com`) is not covered by
a one-level wildcard certificate.

Nested prefixes (`/clients/acme`) work the same way; the proxy just matches two
segments.

## Verifying the wiring

```bash
# Inside the client container: the instance itself.
wget -qO- http://localhost:3000/acme/api/healthcheck     # {"status":"OK"}
wget -qO- http://localhost:3000/api/healthcheck          # 404 — correct

# Through the proxy: the prefix survived.
curl -so /dev/null -w '%{http_code}\n' https://example.com/acme
curl -s https://example.com/acme | grep -c 'href="/acme/static/'
```

A page that loads unstyled while `main.css` returns 200 means the prefix is
being stripped somewhere. A page that loads but whose badges never resolve means
the API calls are reaching the wrong instance.

## Logs worth reading

At startup a prefixed instance says so:

```
{"msg":"serving under a base path","basePath":"/acme"}
```

If that line is missing, `HOMEPAGE_BASE_PATH` never reached the process — check
that the compose service declares it under `environment:` rather than relying on
a host variable being forwarded. If the process refuses to start with `invalid
HOMEPAGE_BASE_PATH`, the value is malformed: it must start with `/`, must not
end with one, and each segment is limited to `A-Za-z0-9._~-`.
