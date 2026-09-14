# Frontend authentication boundary

The current browser client stores access and refresh tokens in Web Storage and
mirrors the access token into a JavaScript-readable cookie for compatibility
with Next.js middleware. Middleware can therefore check token presence only; it
cannot authenticate or revoke the session.

Moving these credentials to `HttpOnly`, `Secure`, `SameSite` cookies requires a
server/BFF session contract that can issue, rotate, validate, and revoke those
cookies. Until that backend contract exists, frontend changes must not imply
that middleware presence checks are an authorization boundary. API endpoints
remain responsible for authentication, tenant isolation, and authorization.
