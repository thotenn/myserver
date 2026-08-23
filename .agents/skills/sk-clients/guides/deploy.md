# Deploying a host that serves several dashboards

Everything here is generic on purpose: hostnames, ports and paths are
placeholders (`example.com`, `<HOST_PORT>`, `/srv/myserver`). Substitute your
own — and keep the real values in your deployment environment, not in this repo.

**One process serves every dashboard.** There is nothing to deploy per client:
no container, no proxy rule, no port, no certificate. A client is a directory
under `dashboards/` in the config tree the process already mounts.

---

## The environment

There is one environment, and it belongs to the root dashboard. A client
dashboard has no environment of its own — which is a feature, since every one of
these variables turns something on that a client has no business reaching.

| Variable | Value | Notes |
|---|---|---|
| `HOMEPAGE_CONFIG_DIR` | `/app/config` | The ROOT dashboard's directory. Clients are read from `dashboards/<slug>/` underneath it; you never point this at one. |
| `HOMEPAGE_BASE_PATH` | usually unset | Moves **every** dashboard under a prefix: `/team` and `/team/acme`. Unset is the tested path. |
| `HOMEPAGE_ALLOWED_HOSTS` | your hostname | `/api/*` refuses a `Host` it does not know. One hostname serves every dashboard. |
| `HOMEPAGE_SCRIPTS_ENABLED` | your call | Root-only by construction: the scripts routes are never registered for a client dashboard, whatever this says. |
| `HOMEPAGE_ALLOW_PRIVATE_HOSTS` | as needed | Governs the widget proxy, which client dashboards cannot reach. |
| `HOMEPAGE_HSTS` | as needed | Only behind HTTPS. |
| `TRUSTED_PROXIES` | your proxy's CIDR | Correct client IPs in the logs, and mandatory for the `trustedHeader` auth provider. |
| `TZ` | yours | Process-wide, so it is yours; a client dashboard in another timezone sees your `datetime` widget in yours. |

## Compose skeleton

One service, one bind mount, and that mount carries every dashboard:

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
```

With `HOMEPAGE_BASE_PATH` set, the healthcheck path moves with everything else
(`/team/api/healthcheck`) — the unprefixed path 404s, and an unprefixed
healthcheck then reports the container unhealthy for the wrong reason.

## The reverse proxy

One rule, for the whole hostname, and the path **must reach the process
intact**: the first segment is how it decides which dashboard you asked for.

| Do | Do not |
|---|---|
| Route the hostname to the one service | Add a rule per client — there is nothing behind it to route to |
| Pass the path through unchanged | Strip anything (`strip_prefix`, `rewrite ^/acme(.*) $1`, `PathPrefix` + `StripPrefix`). A stripped `/acme` is served by the ROOT dashboard |

Adding a client changes nothing here: same hostname, so **no new DNS record, no
new certificate, no new public port, and no new proxy rule**. That is the whole
reason to prefer a path over a subdomain — a second-level subdomain
(`acme.dash.example.com`) is not covered by a one-level wildcard certificate.

## Verifying the wiring

```bash
# The root dashboard, and a client, from inside the container.
wget -qO- http://localhost:3000/api/healthcheck            # {"status":"OK"}
wget -qO- http://localhost:3000/acme/api/healthcheck       # {"status":"OK"}
wget -qO- http://localhost:3000/nobody/api/healthcheck     # 404 — no such dashboard

# Through the proxy: the path survived.
curl -so /dev/null -w '%{http_code}\n' https://example.com/acme
curl -s https://example.com/acme | grep -c 'href="/acme/static/'
```

A page that loads unstyled while `main.css` returns 200 means the path is being
rewritten somewhere. A client page that shows YOUR services means the prefix was
stripped before it reached the process.

## Logs worth reading

At startup the process lists what it is serving:

```
{"msg":"serving a client dashboard","dashboard":"acme","prefix":"/acme"}
{"msg":"config cache initialised","dashboards":3}
```

and warns about a client anyone can read:

```
{"msg":"no allowlist: this client dashboard is PUBLIC to anyone who knows its URL","dashboard":"acme"}
```

A directory that is not being served says why:

```
{"msg":"ignoring a dashboard directory","error":"dashboard slug \"api\" is reserved: it would shadow the /api routes"}
```

Adding one at runtime is logged too, so an alta that did not take is visible:

```
{"msg":"dashboard added","dashboard":"acme","prefix":"/acme"}
```

With `HOMEPAGE_BASE_PATH` set, the prefix is announced as well:

```
{"msg":"serving under a base path","basePath":"/team"}
```

If that line is missing, the variable never reached the process — check that the
compose service declares it under `environment:`. If the process refuses to start
with `invalid HOMEPAGE_BASE_PATH`, the value is malformed: it must start with
`/`, must not end with one, and each segment is limited to `A-Za-z0-9._~-`.

## Failure modes worth recognising

| Symptom | Cause |
|---|---|
| A client URL 404s entirely | No directory of that name under `dashboards/`, or the name is reserved/malformed — check the startup log |
| A client URL serves YOUR dashboard | The proxy stripped the prefix, so the first segment never arrived |
| One client answers 503 everywhere, the rest are fine | That dashboard's `auth.yaml` is unreadable or vanished. Fail-closed by design; the log names it |
| The whole host refuses to start | The ROOT dashboard's `auth.yaml` is broken. A client's never does this |
