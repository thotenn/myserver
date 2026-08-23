# Login, per dashboard

The full schema and the failure table live in
`docs/context/authentication.md`; `add-widget/guides/allowlist.md` covers
setting up a single dashboard. This guide is only about what changes when
**several dashboards share one hostname** — which is where the two traps are.

---

## The allowlist is the switch

There is no `enabled` flag. `auth.yaml` present with at least one address or
domain ⇒ login required for that dashboard. Absent, or a well-formed empty
allowlist ⇒ that dashboard is public.

Each dashboard has its own file in its own config directory, so each has its own
policy: `/acme` can require login while the root dashboard is public, or the
other way round. Neither knows about the other.

```yaml
# config/dashboards/acme/auth.yaml
allowlist:
  emails:
    - you@example.com          # you, so you can see what the client sees
    - contact@acme.example     # the client
google:
  clientId:     "{{HOMEPAGE_VAR_GOOGLE_CLIENT_ID}}"
  clientSecret: "{{HOMEPAGE_VAR_GOOGLE_CLIENT_SECRET}}"
  redirectURL:  "https://example.com/acme/auth/google/callback"
session:
  cookieName: myserver_acme
  secret:     "{{HOMEPAGE_VAR_SESSION_SECRET_ACME}}"
```

## Trap 1 — the cookie name must differ per dashboard

**Symptom:** you can sign in, but the client dashboard bounces back to its login
occasionally, or one dashboard signs you out of the other. It looks random.

**Cause:** the default cookie name is `myserver_session` for every instance. A
session cookie is scoped to its dashboard's path — the root dashboard's is
`Path=/`, which the browser sends to `/acme` as well. Two cookies with the same
name arrive at the client instance, which verifies whichever one it reads first
against its own signing secret; the wrong one fails and the request looks
anonymous.

**Fix:** a distinct `session.cookieName` per dashboard on that hostname
(`myserver_acme`). One line, no downside.

Two dashboards that both live under prefixes (`/home` and `/acme`) never overlap
by path, so the collision cannot happen — but naming them apart anyway costs
nothing and survives someone moving a dashboard to the root later.

Give each dashboard its own `session.secret` too. A shared secret means a cookie
minted for one dashboard verifies at the other, and then only the cookie *name*
and *path* are keeping them apart.

## Trap 2 — Google needs the prefix, registered

`redirectURL` is mandatory and explicit: it is never derived from the `Host`
header, because the dashboard route does not validate `Host` and a forged one
would point the login somewhere else.

Under a base path the callback carries the prefix:

```
https://example.com/acme/auth/google/callback
```

That exact URL has to be listed as an **authorised redirect URI** on the OAuth
client in the Google console, and to match `google.redirectURL` byte for byte.
One OAuth client can hold several redirect URIs, so one client covers every
dashboard — but each new prefixed dashboard is one new entry there.

A mismatch shows up as `redirect_uri_mismatch` from Google, before the user ever
reaches the dashboard.

You can also give a dashboard its own OAuth client (`clientId` +
`clientSecret` + `redirectURL` of its own) when the client wants to own the
consent screen. Nothing in the code assumes they are shared.

## What you do not have to do

- **The OAuth state cookie takes care of itself.** It has to stay at `Path=/`
  (the `__Host-` prefix requires it), so it carries the base path in its name
  instead — `__Host-myserver_oauth_acme`. Two logins in flight under different
  prefixes cannot overwrite each other's state. No configuration.
- **The session cookie is already scoped.** Its `Path` is the dashboard's base
  path, so a client's session is not even sent to `/` or to another prefix.
- **The allowlist is re-checked on every request**, not only at login. Removing
  an address evicts that person on their next click; you do not have to wait for
  a cookie to expire or restart anything.

## Already behind an SSO proxy?

If a proxy in front already authenticates everyone (Cloudflare Access,
Authelia, oauth2-proxy…), use `provider: trustedHeader` instead of Google and
let the proxy assert the identity. The header is only honoured when the peer is
in `TRUSTED_PROXIES`, so an instance reachable directly cannot be spoofed.

Per dashboard this is the same as anything else: its own `auth.yaml`, its own
allowlist. The proxy still has to route the prefix without stripping it.

## Failure modes, briefly

| Situation | Behaviour |
|---|---|
| `auth.yaml` absent | Dashboard is public. No cookies, no redirects, `/auth/*` answers 404. |
| Broken YAML, a previous good policy in memory | Keeps the last good policy, logs loudly. |
| Broken YAML at startup | Refuses to start. |
| The file vanished while login was active | Lockdown: 503 for everything except the healthcheck. |
| Well-formed, empty allowlist | Opens the dashboard. The only way a config failure can mean "public". |
| Authenticated, not on the allowlist | 403 and the denied page. No cookie is issued — authenticating proves who you are, not that you are welcome. |

`HOMEPAGE_AUTH_REQUIRED=true` on an instance that can never afford to be public
turns "no allowlist" into a startup failure and a 503 rather than an open
dashboard. Worth setting on a client instance.

## Checklist for a gated client dashboard

```bash
# Anonymous visit lands on THAT dashboard's login.
curl -so /dev/null -w '%{http_code} %{redirect_url}\n' https://example.com/acme

# The sign-in link and the callback both carry the prefix.
curl -s https://example.com/acme/auth/login | grep -o 'href="/acme/auth/google/start[^"]*"'

# Content is actually gated — not just the page.
for p in / api/services api/widgets api/config/custom.css; do
  curl -so /dev/null -w "$p %{http_code}\n" "https://example.com/acme/$p"
done
```

Then confirm the cookie: signing in at `/acme` must not sign you in at `/`, and
the cookie's `Path` must be `/acme`.
