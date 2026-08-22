# Authentication — email allowlist with Google sign-in

MyServer is public by default. Adding a `config/auth.yaml` with at least one
address turns the whole dashboard into a private one, gated by Google sign-in
and an explicit list of who may enter.

This is an **extension**, not a compatibility change: Homepage never reads
`auth.yaml`, so a config directory carrying one still works with the original
project.

---

## The switch is the allowlist

There is no `enabled: true` flag. The rule is:

| `config/auth.yaml` | Result |
|---|---|
| absent | Public dashboard. No cookies, no redirects, no `/auth/*` routes. |
| present, allowlist empty | Public dashboard, same as above. |
| present, at least one email or domain | Sign-in required for everything. |

Removing every address from the file reopens the dashboard; adding one closes
it again. Neither needs a restart.

### Why an unreadable file does not mean "public"

"Nobody is listed" and "the file could not be read" look identical from the
outside, and treating the second as the first would publish the dashboard
after a typo. The loader therefore distinguishes them:

| Situation | Behaviour |
|---|---|
| File parses, allowlist empty | Public. This is the only way to go public. |
| File parses, allowlist non-empty, config valid | Sign-in required. |
| File is invalid YAML, or fails validation, **and** a working policy was already loaded | The previous allowlist stays in force. The error is logged. |
| File is invalid **and** nothing valid was ever loaded | 503 on everything except `/api/healthcheck`. At startup this is fatal — the process refuses to run. |
| File disappears while sign-in was active | 503 on everything except `/api/healthcheck`. Deleting the file and restarting is the deliberate way out. |

`/api/healthcheck` keeps answering during a lockdown (with 503) so the
container is reported unhealthy rather than appearing hung.

For deployments where being public is never acceptable, set
`HOMEPAGE_AUTH_REQUIRED=true`: the app then refuses to start without an
allowlist, and answers 503 if the policy is ever unavailable.

---

## Setting it up

### 1. Create the OAuth client

In the [Google Cloud console](https://console.cloud.google.com/apis/credentials),
create an **OAuth client ID** of type **Web application**, and register the
callback under *Authorized redirect URIs*:

```
https://dashboard.example.com/auth/google/callback
```

That path is `/auth/google/callback` — three segments, separated by slashes.
Google matches the URI **character for character**, so a typo here (a dot
instead of a slash, a trailing slash, `http` instead of `https`, a different
host) fails with `redirect_uri_mismatch` before the consent screen appears.

*Authorized JavaScript origins* can be left empty: this flow is a plain
server-side redirect and never runs Google's JavaScript SDK.

If the consent screen is still in *Testing*, add the addresses you intend to
allow as test users, or Google refuses them before MyServer ever sees them.

### 2. Put the credentials in the environment

```bash
HOMEPAGE_VAR_GOOGLE_CLIENT_ID=…apps.googleusercontent.com
HOMEPAGE_VAR_GOOGLE_CLIENT_SECRET=GOCSPX-…
```

With Compose, a variable set on the host does **not** reach the container
unless the service names it. `docker-compose.yml` already declares both under
`environment:` with no value, which is what forwards them:

```yaml
    environment:
      - HOMEPAGE_VAR_GOOGLE_CLIENT_ID
      - HOMEPAGE_VAR_GOOGLE_CLIENT_SECRET
```

The same applies on a PaaS: setting the variables in its UI populates the
environment of the *deployment*, and the Compose file is what passes them
through to the container.

### 3. Write `config/auth.yaml`

```yaml
allowlist:
  emails:
    - you@example.com

google:
  clientId:     "{{HOMEPAGE_VAR_GOOGLE_CLIENT_ID}}"
  clientSecret: "{{HOMEPAGE_VAR_GOOGLE_CLIENT_SECRET}}"
  redirectURL:  "https://dashboard.example.com/auth/google/callback"
```

No restart is needed — the file is picked up on the next request.

> **Order matters: variables first, file second.** An `auth.yaml` whose
> placeholders cannot be resolved refuses to start, so writing the file before
> the environment is ready puts the container in a restart loop. That is
> deliberate (a half-configured login must not run), but it is easier to avoid
> than to diagnose.

Confirm the credentials actually arrived before writing the file:

```bash
docker exec <container> printenv | grep HOMEPAGE_VAR_GOOGLE
```

---

## Schema

```yaml
# config/auth.yaml
provider: google              # google (default) | trustedHeader

allowlist:
  emails:
    - person@example.com
  domains:                    # optional: any address at these domains
    - example.com
allowPublicDomains: false     # required to list gmail.com and friends

google:
  clientId:     "{{HOMEPAGE_VAR_GOOGLE_CLIENT_ID}}"
  clientSecret: "{{HOMEPAGE_VAR_GOOGLE_CLIENT_SECRET}}"
  redirectURL:  "https://dashboard.example.com/auth/google/callback"
  hostedDomain: ""            # optional hd= hint for Workspace tenants

trustedHeader:                # only for provider: trustedHeader
  header: "Cf-Access-Authenticated-User-Email"

session:
  secret:     "{{HOMEPAGE_VAR_AUTH_SESSION_SECRET}}"   # optional
  maxAge:     "168h"          # default 7 days
  cookieName: "myserver_session"
  secure:     true            # false only for http://localhost testing

publicPaths: []               # extra paths served without a session
```

### `allowlist`

Matching is case-insensitive and ignores surrounding whitespace. Domains may
be written with or without a leading `@`.

Gmail's dot- and plus-folding is **not** applied: `j.perez@gmail.com` and
`jperez@gmail.com` are the same mailbox to Google but different entries here.
List the address exactly as it is spelled.

`domains:` admits everyone with an address at that domain, which is what you
want for a company domain and never what you want for a public mail provider.
Listing `gmail.com`, `outlook.com`, `proton.me` and similar is refused at
startup unless `allowPublicDomains: true` is also set.

### `google`

`clientId` and `clientSecret` come from an OAuth client of type *Web
application* in the Google Cloud console. Keep them out of the YAML with
`{{HOMEPAGE_VAR_*}}` substitution; if the environment variable is missing the
placeholder survives verbatim and startup fails loudly rather than running
half-configured.

`redirectURL` is **mandatory and explicit**. It is never derived from the
`Host` header: the dashboard route does not validate `Host`, so deriving it
would let a forged header point the login at another domain. Register the same
URL in the Google console.

### `session`

The cookie is stateless: `email | expiry | nonce` signed with HMAC-SHA256, set
`HttpOnly` (mandatory — `custom.js` is arbitrary operator JavaScript on the
same page), `SameSite=Lax` and `Secure` by default. It slides forward once
less than half its life is left.

Leaving `secret` unset generates a random key per process, so sessions do not
survive a restart or redeploy. Set it to keep people signed in across
deployments. Rotating it invalidates every existing cookie — that is the only
"sign out everywhere" available, since there is no session store.

### `publicPaths`

Paths served without a session, on top of the built-in ones. Entries must
start with `/`; a trailing `/` matches by prefix.

`/api/config/custom.css` is gated by default, so the login page does not carry
operator CSS. Add it here if you want your styling on the login screen.

---

## What is gated

Protection is an allowlist of public paths, not a denylist of private ones —
a route added later is protected by default.

| Public | Everything else |
|---|---|
| `/static/*` (the login page's CSS) | `GET /` and every `/api/*` endpoint |
| `/auth/*` (the login flow) | `/api/services`, `/api/widgets`, `/api/bookmarks` |
| `/api/healthcheck` (the container healthcheck) | `/api/services/proxy` — the widget gateway, with upstream credentials |
| anything in `publicPaths` | `/api/scripts/*` — shell execution |

Gating only `GET /` would gate nothing: `/api/services`, `/api/widgets` and
`/api/services/proxy` together rebuild the dashboard from outside.

### How an unauthenticated request is answered

| Request | Response |
|---|---|
| Normal navigation | `302` to `/auth/login?next=…` |
| `HX-Request: true` (widget polling) | `401` + `HX-Redirect: /auth/login` |
| JSON client | `401` + `{"error":"unauthorized"}` |

The HTMX case matters: answering a polling widget with login HTML would make
HTMX paint the login form inside a widget card.

An address that signs in successfully but is not on the allowlist gets `403`
and **no cookie** — authenticating with Google proves who you are, not that
you are welcome. The attempt is logged with the address and the client IP.

---

## The login flow

```
GET /                     no session
  → 302 /auth/login?next=%2F

GET /auth/login           minimal page with a "Sign in with Google" link

GET /auth/google/start    fresh state + nonce (32 bytes each) in an
                          HttpOnly cookie, 10 min
  → 302 accounts.google.com/o/oauth2/v2/auth
        ?scope=openid%20email&state=…&nonce=…&prompt=select_account

GET /auth/google/callback state compared in constant time, cookie consumed
  → POST oauth2.googleapis.com/token        (server to server)
  → validate iss / aud / exp / nonce / email_verified
  → on the allowlist?  no  → 403 /auth/denied, no cookie issued
                       yes → session cookie + 302 to the validated `next`

POST /auth/logout         clears the cookie
```

`scope` is `openid email` and nothing more: an allowlist needs the address,
not the profile or the picture.

The `next` destination is only accepted when it starts with a single `/`.
`//evil.example` and `/\evil.example` are read by browsers as off-site URLs,
so both are rejected, and the check runs again at the moment of use.

### Why there is no JWT library

The ID Token's signature is not verified, deliberately. OIDC Core §3.1.3.7
item 6 permits this when the token is obtained **directly** from the token
endpoint over a TLS channel the client validates — exactly this flow: the
backend POSTs to `oauth2.googleapis.com` over TLS and authenticates with the
client secret. That keeps the dependency list at stdlib only: no JWKS fetch,
no key cache, no JWT dependency.

The claims are still validated, and that is not optional: `iss`, `aud`, `exp`
(60 s skew), `nonce` and `email_verified`.

> If MyServer ever accepts a token from anywhere other than the token endpoint
> — implicit flow, Google One Tap, a generic IdP — signature verification
> becomes mandatory and this shortcut no longer applies.

---

## `provider: trustedHeader`

For deployments already behind an SSO proxy. The identity comes from a header
the proxy sets, and goes through the same allowlist and the same gate:

```yaml
provider: trustedHeader
trustedHeader:
  header: "Cf-Access-Authenticated-User-Email"
allowlist:
  emails: [person@example.com]
```

Common headers: `Cf-Access-Authenticated-User-Email` (Cloudflare Access),
`Remote-Email` (Authelia), `X-Forwarded-Email` (oauth2-proxy).

**The header is honoured only when the immediate peer is listed in
`TRUSTED_PROXIES`** (default `127.0.0.1/8,::1/128`). Otherwise it is ignored
and the request is refused — without that check, any client could set the
header and walk in. No OAuth client, no cookies, no `google:` block needed.

---

## Hot-reload behaviour

`auth.yaml` is watched like every other config file, and the policy is read
per request rather than captured at startup. Consequences:

- Adding an address grants access on their next request.
- Removing one evicts that person on their next request, without waiting for
  their cookie to expire. A valid cookie is not a standing permission.
- Editing the file changes the config hash, so browsers pick up the change.
- A broken edit keeps the previous policy (see the table at the top).

---

## Environment variables

| Variable | Effect |
|---|---|
| `HOMEPAGE_VAR_*` | Substituted into `auth.yaml`. Use for the client id, the secret and the session key. |
| `HOMEPAGE_AUTH_REQUIRED` | `true` refuses to start without an allowlist and answers 503 whenever the policy is unavailable. |
| `TRUSTED_PROXIES` | CIDR list whose members may assert identity headers (`trustedHeader`) and forwarded IPs. |

---

## Troubleshooting

The same entries, in symptom-first order, live in
[`troubleshooting.md`](./troubleshooting.md#authentication-email-allowlist).

### `redirect_uri_mismatch` from Google

The `redirectURL` in `auth.yaml` and the *Authorized redirect URI* in the
Google console are not identical. Compare them character by character:
`/auth/google/callback` (slashes, not dots), same scheme, same host, no
trailing slash. Changes in the Google console can take a few minutes to apply.

### Everything answers 503

The auth policy could not be read. This is the deliberate failure mode — a
config MyServer cannot parse never degrades to a public dashboard. Look at the
log for the reason:

```bash
docker logs <container> 2>&1 | grep -i auth
```

The usual causes are a YAML syntax error in `auth.yaml`, the file having
disappeared (a bind mount that did not come up), or a first-ever policy that
fails validation. Fix the file and the next request recovers; no restart
needed.

### The container restarts in a loop

Startup validation failed. The log line names the field:

```
authentication is configured but unusable; refusing to start
google.clientId still holds an unresolved placeholder — is the environment variable set?
```

The environment variables did not reach the container. See step 2 above — on
Compose, they must be declared under `environment:`. To get the dashboard back
immediately, empty the allowlist (`emails: []`) rather than deleting the file.

### Sign-in works but the dashboard answers 403

The account authenticated with Google but is not on the allowlist. That is the
feature working. The log records the attempt:

```
access denied: email not in allowlist  {"email": "...", "ip": "..."}
```

Check for a typo in `allowlist.emails`. Matching ignores case and surrounding
whitespace, but **not** Gmail's dots: `j.perez@gmail.com` and
`jperez@gmail.com` are the same mailbox to Google and different entries here.

### Everyone is signed out after each deploy

`session.secret` is unset, so a random key is generated at every start. The
log says so at startup. Set it to a fixed value — via `{{HOMEPAGE_VAR_…}}`, or
generated once and stored outside `auth.yaml` so that regenerating the config
does not rotate it.

### The login page loads but signing in never completes

Over plain HTTP the session cookie is dropped: it is `Secure` by default, and
the OAuth state cookie uses the `__Host-` prefix, which browsers refuse
without HTTPS. For local testing over `http://localhost`, set
`session.secure: false` — never in production.

### Removing someone did not lock them out

It should, on their very next request: the allowlist is re-checked per
request, not just at login. If it did not, the file was probably not parsed —
check the log for a retained "last known good" policy, which means your edit
was rejected and the previous allowlist is still in force.

### A widget shows the login page inside its card

That should not happen: HTMX requests get `401` + `HX-Redirect` instead of
login HTML. If you see it, the request is reaching the gate without the
`HX-Request: true` header — check any custom JavaScript issuing it.

---

## Out of scope

Local usernames and passwords · roles and RBAC · other identity providers
(the provider interface is ready for them) · MFA · a user-admin panel · API
tokens for automated clients · per-item permissions (which service each
address may see — a separate piece of work that builds on this one).
