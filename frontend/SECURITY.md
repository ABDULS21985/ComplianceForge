# Frontend authentication boundary

The browser never receives an access or refresh bearer token. Login,
registration, refresh, session lookup, and logout go through same-origin Next.js
route handlers under `/api/auth`. Those handlers exchange credentials with the
Go API and store the returned bearer tokens in `__Host-` cookies with
`HttpOnly`, `Secure`, `SameSite`, and root-path attributes. Successful JSON
responses contain only user and expiry metadata.

Application API calls go through `/api/bff/*`. The BFF adds the access token on
the server, rotates an expired pair, retries the original request once, strips
all upstream `Set-Cookie` headers, and clears the complete local session after
a terminal authentication failure. Pre-BFF `cf_access_token` and
`cf_refresh_token` Web Storage values are purged during migration, and the old
JavaScript-readable cookie is expired by middleware.

State-changing browser requests require a same-origin `Origin`, same-origin
Fetch Metadata when supplied by the browser, and a matching synchronizer token
from `/api/auth/csrf`. The synchronizer cookie is also Secure and HttpOnly; only
the short-lived CSRF proof is returned to same-origin JavaScript.

Public vendor and board invitations are exchanged once through a CSRF-protected
portal session endpoint. The capability then lives in a portal-specific,
path-scoped HttpOnly cookie and is removed from the address bar and browser
history. Subsequent calls use a strict `/api/portal/*` allowlist and never
attach session Authorization headers. Fragment-style invitation links
(`/vendor-portal#token=...`) are supported so edge access logs need never
receive the capability; legacy query links are cleaned immediately after the
page reads them.

## Deployment contract

- Set the server-only `API_INTERNAL_URL` to the API v1 base URL. Local
  development defaults to `http://localhost:8080/api/v1`; Docker uses
  `http://api:8080/api/v1`. User information, query strings, fragments, and
  empty path segments are rejected in this value.
- Do not add a `NEXT_PUBLIC_API_URL` or otherwise expose the internal API base
  to browser bundles.
- Terminate production traffic with HTTPS. The `__Host-` cookie prefix requires
  Secure cookies, no Domain attribute, and `Path=/`.
- Set `APP_ENV=production` and either set `APP_ORIGIN` to the canonical public
  HTTPS origin or set `TRUST_PROXY_HEADERS=true` behind an edge proxy that
  overwrites Host and `X-Forwarded-Proto`. Forwarded values are ignored by
  default. Never enable this switch when clients can reach Next.js directly.
- Local plain-HTTP development requires `APP_ENV=development` and a loopback
  browser origin. It uses isolated `cf_dev_*` HttpOnly cookies because browsers
  reject `__Host-`/`__Secure-` prefixes without Secure. Production and every
  non-loopback origin force Secure cookies even if their URL is accidentally
  HTTP.

Middleware cookie checks are a routing optimization, not an authorization
boundary. The Go API remains responsible for validating the persisted session,
tenant isolation, and every authorization decision.

Refresh coalescing is process-local. A horizontally scaled frontend deployment
includes a short completed-rotation grace window, but must still use affinity
for `/api/*` until refresh coordination is moved to a shared store or the API
offers an idempotent rotation grace window. Logout always clears browser
cookies; upstream revocation is best effort if the API is unavailable, so API
availability and short credential lifetimes remain part of the revocation
control.
