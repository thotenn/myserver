# Login, per dashboard

The full schema and the failure table live in
`docs/context/authentication.md`; `add-widget/guides/allowlist.md` covers
setting up a single dashboard. This guide is only about what changes when
**several dashboards share one hostname**.

The short version: much less than you would expect. The two things that used to
be traps — a shared cookie name, and one Google redirect URI per dashboard — are
now handled by the code. What is left is one rule you have to get right.

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
  # The ROOT dashboard's callback — see below. Not /acme/...
  redirectURL:  "https://example.com/auth/google/callback"
session:
  secret: "{{HOMEPAGE_VAR_SESSION_SECRET_ACME}}"
```

## The one rule: point `redirectURL` at the root dashboard's callback

`redirectURL` is mandatory and explicit — never derived from the `Host` header,
because the dashboard route does not validate `Host` and a forged one would point
the login somewhere else.

**Every dashboard names the same one**, the root dashboard's:

```
https://example.com/auth/google/callback
```

That single URL is the only authorised redirect URI the Google console ever
needs. Adding a client adds nothing there — which is the operational cost this
whole design exists to remove.

It works because the login carries the dashboard it belongs to, inside the
**signed** OAuth state cookie. The callback reads that slug, resolves *that*
dashboard's policy, checks *its* allowlist, and issues *its* session cookie. The
signature is not decoration: the cookie sits in the caller's own browser, and the
slug is what decides which allowlist judges them.

A mismatch shows up as `redirect_uri_mismatch` from Google, before the user ever
reaches the dashboard.

**The exception**, if a client wants to own its own consent screen: give that
dashboard its own `clientId`, `clientSecret` and a `redirectURL` of
`https://example.com/acme/auth/google/callback`, and register that URI too. Its
own callback route exists for exactly this. Nothing else changes.

## What you do not have to do

- **Name the session cookie.** The default already carries the slug —
  `myserver_session` at the root, `myserver_session_acme` for a client — because
  dashboards share a hostname and `Path` alone does not disambiguate two
  same-named cookies. Overriding `session.cookieName` with a name another
  dashboard uses re-creates the problem; leaving it alone is correct.
- **Worry about a shared signing key.** Each dashboard has its own, generated per
  dashboard when `session.secret` is unset. And even if two dashboards ended up
  with the same key, the allowlist is re-checked per request against the
  dashboard being served, so a cookie from one still does not open the other.
  Setting an explicit `session.secret` is still worth it: without one, every
  restart signs everybody out.
- **Manage the OAuth state cookie.** It stays at `Path=/` (`__Host-` requires
  it) and carries the prefix in its name — `__Host-myserver_oauth_acme` — so two
  logins in flight under different dashboards cannot overwrite each other. The
  callback finds the right one by matching the state the provider echoed back.
- **Scope the session cookie.** Its `Path` is the dashboard's prefix, so a
  client's session is not even sent to `/` or to another dashboard.
- **Restart anything after an allowlist edit.** The policy is read per request,
  so removing an address evicts that person on their next click.

## Already behind an SSO proxy?

If a proxy in front already authenticates everyone (Cloudflare Access,
Authelia, oauth2-proxy…), use `provider: trustedHeader` instead of Google and
let the proxy assert the identity. The header is only honoured when the peer is
in `TRUSTED_PROXIES`, so an instance reachable directly cannot be spoofed.

Per dashboard this is the same as anything else: its own `auth.yaml`, its own
allowlist. The proxy still has to pass the path through without stripping it —
the first segment is how the process knows which dashboard was asked for.

## Failure modes, briefly

Everything below is **per dashboard**: a client in lockdown answers 503 on its
own prefix and nowhere else, and the rest of the host keeps serving.

| Situation | Behaviour |
|---|---|
| `auth.yaml` absent | Dashboard is public. No cookies, no redirects, `/auth/*` answers 404. A client dashboard in this state is warned about at startup. |
| Broken YAML, a previous good policy in memory | Keeps the last good policy, logs loudly. |
| Broken YAML at startup, ROOT dashboard | Refuses to start. |
| Broken YAML at startup, CLIENT dashboard | The host starts; that dashboard answers 503 and says so on every request. One client's typo is not everyone's outage. |
| The file vanished while login was active | Lockdown: 503 for everything on that dashboard except the healthcheck. |
| Well-formed, empty allowlist | Opens the dashboard. The only way a config failure can mean "public". |
| Authenticated, not on the allowlist | 403 and the denied page. No cookie is issued — authenticating proves who you are, not that you are welcome. |
| A session from another dashboard, renamed by hand | Rejected: different signing key, and the allowlist is re-checked anyway. |

`HOMEPAGE_AUTH_REQUIRED=true` turns "no allowlist" into a refusal rather than an
open dashboard. It is process-wide, so it applies to every dashboard at once: a
client without an `auth.yaml` then answers 503 instead of being public.

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

Then confirm the cookie: signing in at `/acme` must not sign you in at `/`, the
cookie must be named `myserver_session_acme`, and its `Path` must be `/acme`.
