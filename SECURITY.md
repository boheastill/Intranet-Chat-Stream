# Security model & threat notes

ICS is a **single-user personal message bus**. It is not multi-tenant, and the
whole design assumes one trusted operator. This file records what it does and
does not defend against, so nobody mistakes it for something stronger.

## Secrets

`config.json` holds three secrets and is **never committed** (`.gitignore`d):

| field | role |
|---|---|
| `token` | bearer credential for every API call |
| `password` | fallback login password |
| `login_key` | URL "knock" (`?key=…`) that reveals the password form |

Rotate all three if any of them is ever exposed:

```bash
ssh <server> 'python3 - <<PY
import json; p="/home/admin/ics/config.json"; d=json.load(open(p))
d["token"]="<new>"; d["password"]="<new>"; d["login_key"]="<new>"
json.dump(d, open(p,"w"), indent=2)
PY'
ssh <server> 'sudo systemctl restart ics'
```

Rotating the token invalidates every existing session (clients re-login).

## What is defended

- **Token auth on every API route** except `/`, `/index.html`, `/static/*`
  and `/api/login`.
- **Constant-time** comparison for both the token and the login password.
- **Exponential backoff** on failed logins, per source IP *and* globally, so
  rotating IPs does not defeat it. Correct credentials never wait.
- **Path traversal**: `getSafeFilePath` rejects `..`, resolves to an absolute
  path, and requires a path separator after the storage root.
- **Channel names** are whitelisted (`[A-Za-z0-9._-]{1,64}`), rejecting dot-only
  names and the reserved `.chunks` directory.
- **Uploaded files never render**: `/api/download/` always responds with
  `application/octet-stream` + `Content-Security-Policy: default-src 'none'; sandbox`.
  This route is same-origin with the app, so serving an uploaded `.html` as
  `text/html` would let its script read `ics_token` from localStorage.
- **Filenames are never interpolated into HTML**; the UI assigns them via
  `textContent`.
- **HSTS** is sent when the request arrived over TLS, so a plain `http://` link
  cannot downgrade the token or password to cleartext.
- Security headers: `nosniff`, `X-Frame-Options: DENY`, `no-referrer`,
  restrictive `Permissions-Policy`.

## Observability

Every login outcome is logged (success, wrong factor, rate-limited, rejected as
malformed) with the user agent and the submitted value's character/byte counts —
never the secret itself. Every 5xx and every 4xx on a mutating request is logged
from one middleware, so a failed write is traceable without each handler having
to remember. Panics are logged with a stack and answered with 500 rather than
dropping the connection.

Deliberately not logged: routine 404/405 on GET, successful reads, and secret
values. A public host receives constant scanner traffic; logging it would grow
the journal without bound, which is the same failure the request log was meant
to prevent.

## What is NOT defended

Be honest about these before storing anything sensitive:

- **No encryption at rest.** Messages and uploads sit as plain files under
  `files/` on the server. Anyone with shell or disk access reads everything.
- **No per-message access control.** One token = full read/write/delete on all
  channels.
- **No audit trail beyond stdout logs**, which the host controls.
- **Anyone with the token is fully trusted.** There is no session expiry, no
  device binding, no revocation list.
- **The token is a bearer string in `localStorage`.** Any XSS in the app would
  exfiltrate it. That is why the download route is locked down.
- **Cloudflare sees the traffic** (TLS terminates there).
- **No E2E encryption.** The operator of the server can read everything.

## Reporting

This is a personal project with no bug bounty. If you find something, open an
issue without exploit details, or contact the maintainer directly.

## Deployment hardening (Cloudflare)

The zone `bohea.us` fronts ICS with a Cloudflare Tunnel, so no origin IP is
published. These settings are part of the security model and should be verified
after any zone change:

| setting | required value | why |
|---|---|---|
| SSL mode | `strict` | tunnel terminates TLS; never fall back to flexible |
| Always Use HTTPS | `on` | otherwise `http://` serves the app and leaks the token/password in cleartext |
| Min TLS version | `1.2` | drop obsolete protocols |
| Security level | `medium`+ | managed WAF rules stay active |
| Rate limit rule | 3 requests / 10 s per IP on `/api/login` | free-plan limits: one rule, 10 s period |

Check them with:

```bash
curl -s -H "Authorization: Bearer $CF_TOKEN" \
  "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/settings/always_use_https"
```

**Note:** `web.bohea.us` is additionally behind Cloudflare Access (Zero Trust).
`flow.bohea.us` deliberately is *not* — the app has its own token auth, and an
Access redirect would break the non-browser clients (MCP `ics-core`, curl).
