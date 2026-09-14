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
the short-lived CSRF proof is returned to same-origin JavaScript. Public vendor
and board invitations remain token-authenticated, but their API calls use a
separate strict allowlist under `/api/portal/*`; session Authorization headers
are never attached to those calls.

## Deployment contract

- Set the server-only `API_INTERNAL_URL` to the API v1 base URL. Local
  development defaults to `http://localhost:8080/api/v1`; Docker uses
  `http://api:8080/api/v1`.
- Do not add a `NEXT_PUBLIC_API_URL` or otherwise expose the internal API base
  to browser bundles.
- Terminate production traffic with HTTPS. The `__Host-` cookie prefix requires
  Secure cookies, no Domain attribute, and `Path=/`.
- Preserve the external Host and scheme presented to Next.js so exact-origin
  CSRF checks use the browser-visible origin.

Middleware cookie checks are a routing optimization, not an authorization
boundary. The Go API remains responsible for validating the persisted session,
tenant isolation, and every authorization decision.

Refresh coalescing is process-local. A horizontally scaled frontend deployment
must use affinity for `/api/*` until refresh coordination is moved to a shared
store or the API offers an idempotent rotation grace window. Logout always
clears browser cookies; upstream revocation is best effort if the API is
unavailable, so API availability and short credential lifetimes remain part of
the revocation control.
