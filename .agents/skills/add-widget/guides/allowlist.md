# Guide — Email allowlist with Google sign-in

How to put a MyServer dashboard behind a login, end to end: creating the OAuth
client in Google, wiring the credentials, writing `config/auth.yaml`, and the
day-to-day of adding and removing people.

Reference documentation: `docs/context/authentication.md`. This guide is the
operational version — what to do, in order, and what to say to the user.

---

## When this applies

The user asks for any of: "only I should see this", "add a login", "restrict
by email", "allowlist", "require Google sign-in", "my dashboard is public and
I don't want it to be".

**Before starting, ask one question:** *is there already an auth layer in front
of the dashboard* (Cloudflare Access, Authelia, oauth2-proxy, Tailscale,
Authentik)?

- **Yes** → use [`provider: trustedHeader`](#alternative-already-behind-sso).
  Five lines of YAML, no OAuth client, no cookies. Much less work.
- **No** → continue with Google sign-in below.

---

## The mental model (explain this first)

There is no `enabled: true` switch. **The allowlist is the switch:**

| `config/auth.yaml` | Result |
|---|---|
| does not exist | Dashboard is public, exactly as before |
| exists, `emails: []` | Dashboard is public |
| exists, one or more addresses | Login required for everything |

Two consequences worth telling the user up front:

1. **Adding or removing an address takes effect on the next request.** No
   restart. Removing someone kicks them out immediately — a valid cookie is
   not a standing permission.
2. **To go back to public, empty the list — never delete the file.** A file
   that disappears while sign-in is active makes MyServer answer 503 on
   everything, because "I deleted it" and "the mount broke" are
   indistinguishable, and guessing wrong would publish the dashboard.

---

## Step 1 — Create the OAuth client in Google

This happens in the **Google Cloud console** (`console.cloud.google.com`) —
not the Play Console, which is for Android apps. The user must do this part
themselves; you cannot do it for them. Walk them through it:

1. Go to **console.cloud.google.com** and create a project (or pick one).
   Name is irrelevant — "MyServer Dashboard" is fine.

2. **APIs & Services → OAuth consent screen**:
   - User type: **External** (unless they have a Google Workspace org and only
     want people inside it — then **Internal**, and no test users are needed).
   - App name, support email, developer email: whatever they want.
   - **Scopes: none needed.** Do not add any. The flow requests `openid` and
     `email`, which are always available and do not require declaring scopes.
   - **Test users**: while the consent screen is in *Testing*, only addresses
     listed here can sign in. **Add every address that will go in the
     allowlist**, or Google will reject them before MyServer ever sees the
     request. Alternatively publish the app.

3. **APIs & Services → Credentials → Create credentials → OAuth client ID**:
   - Application type: **Web application**.
   - **Authorized redirect URIs** → Add:

     ```
     https://dashboard.example.com/auth/google/callback
     ```

     Replace the host with the user's real dashboard URL.

   - **Authorized JavaScript origins**: leave empty. This flow is a plain
     server-side redirect and never loads Google's JavaScript SDK. (Adding one
     is harmless.)

4. Copy the **Client ID** (`…apps.googleusercontent.com`) and the **Client
   secret** (`GOCSPX-…`).

> ### ⚠️ The redirect URI is where this goes wrong
>
> The path is `/auth/google/callback` — **three segments separated by
> slashes**. Google compares the whole URI character for character. Every one
> of these fails with `redirect_uri_mismatch` before the consent screen even
> appears:
>
> | Wrong | Why |
> |---|---|
> | `/auth/google.callback` | dot instead of a slash |
> | `/auth/callback` | missing the `google` segment |
> | `/auth/google/callback/` | trailing slash |
> | `http://…` | scheme must match what the user actually browses |
> | a different host | must be the host they type in the browser |
>
> Read it back to the user character by character before moving on.

---

## Step 2 — Put the credentials in the environment

**Never write the client secret into `auth.yaml`.** It goes in the
environment, and the YAML references it:

```bash
HOMEPAGE_VAR_GOOGLE_CLIENT_ID=…apps.googleusercontent.com
HOMEPAGE_VAR_GOOGLE_CLIENT_SECRET=GOCSPX-…
```

Where to set them depends on the deployment:

| Deployment | Where |
|---|---|
| plain `docker compose` | a `.env` next to `docker-compose.yml` |
| a PaaS (Dokploy, Coolify, CapRover…) | the platform's environment / secrets UI |
| bare binary | the process environment (systemd unit, shell export) |

**On Compose, a variable set on the host does not reach the container unless
the service names it.** `docker-compose.yml` already declares both with no
value, which is what forwards them:

```yaml
    environment:
      - HOMEPAGE_VAR_GOOGLE_CLIENT_ID
      - HOMEPAGE_VAR_GOOGLE_CLIENT_SECRET
```

If the user is on a custom or older compose file, check that these two lines
are there. Setting the variables in a PaaS UI populates the *deployment*
environment; the Compose file is still what passes them through.

**Verify before going further:**

```bash
docker exec <container> printenv | grep HOMEPAGE_VAR_GOOGLE
```

Two lines with values → good. Nothing → stop and fix this first; do not write
`auth.yaml` yet.

---

## Step 3 — Write `config/auth.yaml`

```yaml
# config/auth.yaml
allowlist:
  emails:
    - owner@example.com

google:
  clientId:     "{{HOMEPAGE_VAR_GOOGLE_CLIENT_ID}}"
  clientSecret: "{{HOMEPAGE_VAR_GOOGLE_CLIENT_SECRET}}"
  redirectURL:  "https://dashboard.example.com/auth/google/callback"
```

That is the whole minimum file. `redirectURL` must be **identical** to what
was registered in step 1.

Optional, and worth adding for anything long-lived:

```yaml
session:
  secret:     "{{HOMEPAGE_VAR_AUTH_SESSION_SECRET}}"
  maxAge:     "168h"
  secure:     true
```

Without `session.secret`, MyServer generates a random key at every start, so
**everyone is signed out on each restart or redeploy**. Set it (any long random
string: `openssl rand -hex 32`) if that would annoy the user.

> **Order matters: environment first, file second.** An `auth.yaml` whose
> `{{HOMEPAGE_VAR_*}}` placeholders cannot be resolved makes the process refuse
> to start — a half-configured login must not run. Written in the wrong order,
> the container ends up in a restart loop.

### Confirm it worked

```bash
docker logs <container> 2>&1 | grep -i auth
```

Expect `authentication enabled` with the address count. Then open the
dashboard: it should redirect to the login page.

Ask the user to test **both** halves:

1. Sign in with an address **on** the list → the dashboard loads.
2. Sign in with any other Google account → **403, access denied**.

The second test is the one people skip, and it is the one that proves the
allowlist is actually being enforced rather than just the login.

---

## Day-to-day tasks

### Add someone

Append to `allowlist.emails`. Nothing else, no restart:

```yaml
allowlist:
  emails:
    - owner@example.com
    - teammate@example.com      # ← new
```

If the Google consent screen is still in *Testing*, the new address **also**
has to be added as a test user in the Google Cloud console, or Google blocks
them before MyServer sees anything.

### Remove someone

Delete their line. They are locked out on their very next request — not when
their cookie expires.

Removing the last address makes the dashboard public again. If that is not the
intent, keep at least one.

### Let in a whole company domain

```yaml
allowlist:
  emails:
    - contractor@othercompany.example
  domains:
    - mycompany.example        # anyone @mycompany.example
```

Domains may be written with or without a leading `@`. `emails` and `domains`
are additive.

**Public mail providers are refused.** `gmail.com`, `outlook.com`, `yahoo.com`,
`proton.me`, `icloud.com` and similar under `domains:` would admit anyone on
the internet who can register an address, so MyServer refuses to start with
one. If the user genuinely means it, they must be explicit:

```yaml
allowPublicDomains: true
```

Push back before writing that line — it almost always means they wanted
`emails:` instead. Listing individual Gmail addresses under `emails:` is
completely fine and is the normal case; the guard is only about `domains:`.

### Go back to public

```yaml
allowlist:
  emails: []
```

Keep the file, empty the list. **Do not delete the file** — that gives 503 on
everything, not a public dashboard.

### Open one path without a session

```yaml
publicPaths:
  - /api/config/custom.css
```

`custom.css` is gated by default, so the login page does not carry the
operator's CSS. Opening it makes the login page match the dashboard's look.

---

## Alternative: already behind SSO

If a reverse proxy already authenticates users, skip the whole OAuth dance and
read the identity it asserts:

```yaml
provider: trustedHeader
trustedHeader:
  header: "Cf-Access-Authenticated-User-Email"
allowlist:
  emails:
    - owner@example.com
```

Common headers:

| Proxy | Header |
|---|---|
| Cloudflare Access | `Cf-Access-Authenticated-User-Email` |
| Authelia | `Remote-Email` |
| oauth2-proxy | `X-Forwarded-Email` |
| Authentik | `X-Authentik-Email` |

No `google:` block, no client, no cookies. The header is trusted **only when
the immediate peer is in `TRUSTED_PROXIES`** (default `127.0.0.1/8,::1/128`);
if the proxy reaches MyServer from another address, add its CIDR there or
every request is refused.

Never suggest this without a proxy actually in front: with nothing enforcing
it, anyone could set the header themselves — which is exactly why the
`TRUSTED_PROXIES` check exists.

---

## What is protected

Everything except `/static/*`, `/auth/*`, `/api/healthcheck` and anything in
`publicPaths`. That includes `/api/services`, `/api/widgets`,
`/api/services/proxy` and `/api/scripts/*`.

This matters for two questions users ask:

- *"Can I still hit the API from a script?"* — Not anonymously. There are no
  API tokens; a client would need a session cookie. Automation that needs
  `/api/*` should either run on a `publicPaths` entry or stay off the
  allowlist path entirely.
- *"Will my healthcheck break?"* — No, `/api/healthcheck` stays public
  deliberately.

---

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `redirect_uri_mismatch` | `redirectURL` ≠ the URI registered in Google | Compare character by character; console changes take minutes to apply |
| Container restart loop | Env vars never reached the container | `docker exec … printenv \| grep HOMEPAGE_VAR_GOOGLE`; declare them under `environment:` |
| 503 on everything | The policy could not be read | `docker logs … \| grep -i auth`; fix the YAML, or set `emails: []` to reopen. Do **not** delete the file |
| Login works, then 403 | Address authenticated but not on the list | Check for typos. Case and whitespace are ignored; Gmail dots are **not** (`j.perez@` ≠ `jperez@`) |
| Google blocks the account before MyServer | Consent screen in *Testing* | Add the address as a test user, or publish the app |
| Signed out after every deploy | `session.secret` unset → random key each start | Set it via `{{HOMEPAGE_VAR_…}}` |
| Login page loads, sign-in never completes | Plain HTTP: `Secure` and `__Host-` cookies are dropped | HTTPS in production; `session.secure: false` only for local testing |
| Login form appears inside a widget card | An HTMX request without the `HX-Request` header | Check custom JavaScript |

---

## Rules — never violate

1. **Never put the client secret in `auth.yaml`.** Always
   `{{HOMEPAGE_VAR_GOOGLE_CLIENT_SECRET}}`.
2. **Never tell a user to delete `auth.yaml` to go public.** Empty the list.
   Deleting it gives 503 on everything.
3. **Environment variables first, then the file.** The reverse is a restart
   loop.
4. **Never add `allowPublicDomains: true` without saying what it does.** It
   opens the dashboard to everyone with an address at that provider.
5. **Never suggest `trustedHeader` without a proxy in front of the dashboard**,
   and never without the `TRUSTED_PROXIES` check being satisfied.
6. **Always have the user test a non-allowlisted account too.** A login that
   lets everybody in looks identical to one that works.
7. `redirectURL` is explicit on purpose — never suggest deriving it from the
   `Host` header or making it "automatic".
