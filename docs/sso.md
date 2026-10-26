# SSO / enterprise identity

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

This page describes how ai-memory works with a central identity provider (IdP)
that issues OpenID Connect (OIDC) tokens. It covers the developer sign-in flow,
how the stored token is used and refreshed, what the server does and does not
check, how identity reaches the server through a trusted proxy, and how roles
and sessions behave.

Read it together with:

- [security.md](security.md) for authentication in general.
- [users.md](users.md) for the resolution tiers, trusted proxy identity, and
  identity keys.
- [cli.md](cli.md) for the full `auth` command reference.
- [api-reference.md](api-reference.md) for which routes accept which credential.

## Summary

| Question | Answer |
|---|---|
| Supported sign-in protocol | OIDC device authorization grant, against any issuer that publishes a standard discovery document. |
| Who signs in | Each developer, once per data directory, from the CLI. |
| What is stored locally | Access token, refresh token, access-token expiry, issuer, client id, and token endpoint, in the shared `auth.json` file under the `oidc` key. |
| Does the server validate the OIDC token? | No. The server does not check the token's signature, issuer, audience, expiry, or claims. It treats the bearer value as an opaque credential. |
| Can a raw OIDC access token call the server? | No, not on a server that has a root bearer or human login configured. It receives 401 unless a gateway translates the identity (see below). |
| How does SSO identity reach the server? | Only through a trusted proxy that holds a dedicated proxy bearer and asserts the user in `X-Memory-Actor-*` headers. |
| Is there a server-side SSO session? | No. Each request is authenticated on its own. |

## What this is and is not

- It identifies the **developer** to the CLI and to native lifecycle hooks, and
  it lets the CLI call the server as that developer's stored token.
- It is **not** a login gate for the server's administrative surface. Admin
  routes stay root-only in multi-user mode (see [users.md](users.md)).
- The server does not validate OIDC tokens against the IdP, does not fetch the
  IdP's signing keys, and does not check roles or scopes carried by a token.
  Any check of who a token belongs to has to happen in a gateway in front of the
  server.
- Logging out is local. It removes the stored token from the developer's machine
  and does not revoke the token at the IdP.

## Sign-in flow

The command is:

```bash
ai-memory auth login oidc-device \
  --issuer "https://issuer.example.com/realms/team" \
  --client-id "ai-memory-cli"
```

Both `--issuer` and `--client-id` are required (`--timeout-secs` defaults to 600). The client is a public client:
no client secret is sent at any step.

**Transport rules (confirmed in the original).**

- Every credential-bearing URL must use `https`: the issuer, the discovery URL, the device
  authorization endpoint and the token endpoint (including the token endpoint saved in the token
  file, re-checked on every refresh). Plain `http` is accepted only when the host is `localhost`
  (any case), or an IPv4/IPv6 loopback address (`127.0.0.1`, `[::1]`, and other loopback
  addresses). Any other scheme or host fails before a request is sent, with
  `OIDC <what> "<url>" must use https (plain http is accepted only on localhost, 127.0.0.1 or ::1)`
  (an unparseable URL gives `… is not a valid URL: <detail>`).
- The login client follows **no redirects** (a 3xx is a non-success status), has a 60-second
  timeout per request and sends `User-Agent: ai-memory/<version>`. The refresh client also follows
  no redirects, so a refresh token is never forwarded.
- Successful response bodies are read up to 16 MiB. Error bodies shown to the user are read up to
  1 MiB, passed through the secret scrubber and cut to 1 024 bytes with an ellipsis.

1. **Discovery.** The client requests (GET) the discovery document at the issuer URL
   with a trailing `/.well-known/openid-configuration` suffix. All trailing slashes on
   the issuer are removed first. Any non-success status fails the login with
   `OIDC discovery failed (<status>) for <url>: <body>`. The
   client uses only two fields from the document, both required strings: the device authorization
   endpoint and the token endpoint; both are then checked against the https rule above.
   The `issuer` stored later is the text typed on the command line, not a field of the document.
2. **Device authorization.** The client posts (form-encoded) `client_id` and the scope
   `openid profile offline_access` to the device authorization endpoint. Any
   non-success status fails the login with `device authorization request failed (<status>): <body>`.
   The response must contain `device_code`, `user_code` and `verification_uri`; `verification_uri_complete`,
   `interval` and `expires_in` are optional.
3. **User action.** If the response includes a complete verification URI, the
   CLI prints `Open this URL: <complete uri>`. Otherwise it prints `Open this URL: <verification uri>`
   and `Enter code: <user code>`. Then it prints `Waiting for authorization...`.
4. **Polling.** The CLI posts (form-encoded) `grant_type=urn:ietf:params:oauth:grant-type:device_code`,
   `device_code` and `client_id` to the token endpoint.
   - The first poll happens immediately.
   - The wait between polls is the server's `interval` (default 5 seconds when absent, at least 1 second) plus a
     3-second safety margin, fixed once at the start; each `slow_down` response adds 5 seconds to it permanently.
   - Order of each iteration: check the time limit; poll; act on the outcome; sleep the wait. The sleep is
     not shortened to fit the limit, so the failure by timeout can arrive up to one wait after the limit.
5. **Time limit.** Polling stops (checked before each poll) at the smaller of `--timeout-secs` (default
   600) and the device code's `expires_in` (default 600 when absent, at least 1 second). Message:
   `timed out waiting for oidc-device authorization`.
6. **Outcomes.**

   | Token endpoint response | Result |
   |---|---|
   | `authorization_pending` | Keep polling. |
   | `slow_down` | Keep polling with a longer wait. |
   | Success | Store the token (step 7). |
   | `access_denied` | Login fails: `oidc-device authorization denied`. |
   | `expired_token` | Login fails: `oidc-device code expired before authorization`. |
   | Any other error code | Login fails: `oidc-device authorization failed: <code>`. |
   | Error body with no recognizable code (not JSON, or JSON without `error`) | Login fails with `oidc-device authorization failed: <body>`. |
   | Time limit reached | Login fails with the timeout message above. |

   A 2xx response is a success; every non-2xx response is interpreted by the `error` field of its JSON body
   (a 2xx body that is not the expected token JSON is a read error and aborts the login).

   A transport error while polling aborts the login immediately.

7. **Store.** The token response must include a refresh token. The refresh
   token is requested through the `offline_access` scope. If it is missing, the
   login fails and nothing is written. The access-token expiry is computed from
   `expires_in`, with a default of 300 seconds when it is absent. The entry is
   written under the `oidc` key of the shared `auth.json` file in the data
   directory, with type `oauth`. Entries for other providers in the same file are
   preserved.

The file is written atomically (temporary file `auth.json.tmp` beside it, fsync, rename over the old file). On Unix systems
it is created with owner-only permissions (`0600`); on Windows there are no mode bits and the protection is the
data directory's own folder permissions. The CLI prints `oidc-device: logged in`, `issuer: <issuer>` and
`token file: <path>`. It does not print the tokens.

**File format.** `<data_dir>/auth.json` is one pretty-printed JSON object with one entry per provider. The
`oidc` entry is `{"type": "oauth", "access": "<token>", "refresh": "<token>", "expires": <ms since epoch>,
"issuer": "<issuer as typed>", "client_id": "<id>", "token_endpoint": "<url>"}` (flat keys). Rules for the
shared file: a missing file means no token; a file that is not valid JSON, or not a JSON object, is an error
(`parse auth file …` / `must contain a JSON object`) and nothing is written; an entry whose `type` is not
`oauth` is ignored as absent; an `oauth` entry with missing fields is an error (`parse oidc auth entry: …`).
Saving keeps the other keys; removing the last remaining key deletes the file itself. The path is the same
file other providers use; the original has no setting to move it.

## What the client reads from the IdP

| Source | Fields read | Notes |
|---|---|---|
| Discovery document | Device authorization endpoint, token endpoint | Other fields are ignored. |
| Device authorization response | Device code, user code, verification URI, complete verification URI, `interval`, `expires_in` | |
| Token response | Access token, refresh token (optional), `expires_in` (optional) | |

The client does **not** read the ID token, and it does not decode the access
token. The subject, username, email, roles, and session identifiers in the
token are not read by ai-memory. Access-token expiry comes from `expires_in`, not
from any expiry claim inside the token.

## Token lifecycle

### Refresh

- A stored token is considered stale when the current time plus 60 seconds is at
  or after its expiry. The margin is 60 seconds.
- Refresh posts (form-encoded) `grant_type=refresh_token`, `refresh_token` and `client_id`, from the stored
  entry, to the stored token endpoint (which must pass the https rule, else the refresh fails before any
  request). No client secret is sent, no redirect is followed. The request
  timeout is 300 seconds. A non-success status is the failure
  `OIDC refresh failed (<status>): <body>. Run `ai-memory auth login oidc-device` again.`, and a success body
  without an access token is a failure too. The new expiry is `now + expires_in` (300 s when absent).
- On success, the new access token replaces the old one. If the response
  includes a new refresh token, it replaces the stored one. Otherwise the previous
  refresh token is kept. The updated token is saved before it is used.
- If the refresh is rejected, the existing access token is used for the request.
  The failure is not surfaced to the caller.
- If the refreshed token cannot be saved, the request uses the old access token.
  The rotated token is not used, so it stays consistent with the file.

### Where the token is used

The stored token is the bearer for two kinds of client traffic. A static bearer
always takes precedence over it.

| Client path | Bearer order |
|---|---|
| Native lifecycle hooks | Explicit auth token passed to the hook, then the install's persisted token, then the stored OIDC token, then no bearer. |
| CLI commands that call the server (for example `status`, `search`, `read-page`, `write-page`, `backup`, `embed`) | Static bearer from `AI_MEMORY_AUTH_TOKEN` or `[auth].bearer_token`, then the stored OIDC token, then no bearer. |

Hook events are first written to a local spool. An event that uses the OIDC
token records only its auth mode. The access token is resolved, and refreshed if
needed, when the event is sent. Within one send run the token is resolved once
and reused.

A hook profile with its own server URL and token never uses the stored OIDC
token, and the install's stored token is never sent to a profile's server.

If no token can be loaded at send time, the request carries no bearer. "Cannot be loaded" includes a missing
file, a missing `oidc` entry **and** a file that is unreadable or malformed: the resolver swallows load errors
and returns no bearer (unlike `auth status`, which reports the error). An empty static token counts as no static
token. For CLI commands the static source is the configured bearer (`AI_MEMORY_AUTH_TOKEN` or `[auth].bearer_token`);
the OIDC token file is always `<data_dir>/auth.json`.

There is no lock around refresh: two processes that find the token stale at the same time may both refresh, and
if the IdP rotates refresh tokens the second may be rejected; the loser then falls back to the access token it
already has. A spooled event with auth mode `oidc` is sent with whatever the resolver returns at drain time; a
401 on such an entry is a real rejection and is not retried with another credential. Static-token entries retry
once with the drain process's current `AI_MEMORY_AUTH_TOKEN` when it differs from the rejected one.

### Status and logout

- `ai-memory auth status` reports on every token-backed provider (the OpenAI OAuth and Copilot sections come
  from other features). The OIDC section prints, line by line, `oidc-device: logged in`, `issuer: <issuer>`,
  `expires in: <human duration, or "expired">`, `token file: <path>`; or `oidc-device: not logged in` followed by
  `token file: <path>`. A damaged token file makes the command fail with the load error. It does not print secrets.
- `ai-memory auth logout oidc-device` removes only the `oidc` entry from the
  shared file (the file itself disappears if it was the last entry) and prints `oidc-device: logged out` and
  `token file: <path>`; it succeeds even when nothing was logged in. It does not contact the IdP.
- `auth login oidc-device` rejects `--github-token` (`--github-token applies only to `auth login copilot``) and
  requires both options (`--issuer is required for `auth login oidc-device``, `--client-id is required …`).

## How the server treats a bearer

Machine routes (`/mcp`, `/hook`, `/handoff`, and `/workstream/*`) accept a
`Authorization: Bearer` credential only. Browser login cookies never authenticate
these routes.

For each bearer value, the server tries these checks in order. The first match
decides the outcome.

1. **Root bearer.** The value equals the configured root bearer, compared in
   constant time. The request is root. Identity comes from the root actor
   settings (`[auth].root_username`, `root_email`, `root_name`), if set.
2. **Trusted proxy bearer.** The value equals `[auth].actor_proxy_bearer_token`.
   The identity comes from the `X-Memory-Actor-*` headers (see
   [Trusted proxy identity](#trusted-proxy-identity)).
3. **Database user credential.** The value's hash matches an active API
   credential. The request is a user, attributed to the owning user.
4. **No match.** When a root bearer or human login is configured, the server
   returns `401` with a Bearer challenge (`WWW-Authenticate: Bearer` with
   `error="invalid_token"`). When neither is configured, the server does not
   check bearers at all: the request is anonymous and the bearer is ignored.

A raw OIDC access token matches none of the first three checks, so it gets `401`
on a secured server. The server does not verify the token itself, so the
developer's OIDC token works against the server only when a gateway replaces it
with a credential the server accepts.

## Trusted proxy identity

This is the supported way to put SSO in front of the server. The detailed rules
are in [users.md](users.md#trusted-proxy-identity). In summary:

- The gateway validates the developer's OIDC credential itself. It then calls
  the server with the proxy bearer and sets the identity headers. The server
  never sees the user's OIDC credential.
- The gateway must replace any `X-Memory-Actor-*` header sent by the client.
  Appending instead of replacing is not safe.
- Headers read in this path are `X-Memory-Actor-User`, `X-Memory-Actor-Issuer`,
  `X-Memory-Actor-Sub`, `X-Memory-Actor-Agent`, `X-Memory-Actor-Client`, and
  `X-Memory-Actor-Session-Id`. Without a valid proxy bearer, they are ignored.
- Each of these headers may appear once. A repeated header, or a value containing
  a comma, returns `400` (`ambiguous X-Memory-Actor-* header: the proxy must replace client-supplied actor
  headers, not append to them`). Values are trimmed; a value that is blank or not visible ASCII counts as absent.
  `name` and `email` of the actor are not read from headers. The other 400 texts are
  `trusted proxy must assert X-Memory-Actor-User or both X-Memory-Actor-Issuer and X-Memory-Actor-Sub` and
  `trusted proxy must assert X-Memory-Actor-Issuer and X-Memory-Actor-Sub together`.
- The request must name a person. Either `X-Memory-Actor-User` is set, or both
  `X-Memory-Actor-Issuer` and `X-Memory-Actor-Sub` are set. A partial OIDC pair
  returns `400`. A request with no identity returns `400`.
- Blank values count as absent.
- The server does not check the asserted issuer or subject against any list of
  IdPs. Trust comes from the proxy bearer and from network reachability, so the
  server must be reachable only through the proxy.

### Roles from proxy identity

- A proxy-asserted identity is a **user** by default.
- It is **root** only when the issuer and subject exactly match the configured
  `[auth].root_issuer` and `[auth].root_subject` pair. A username never grants
  root.
- A user-level request can use normal routes but not `/admin/*`.
- A proxy identity has no account row, so none of the per-request account bookkeeping happens (no
  `last_seen_at`, no author id on pages or audit rows), and it carries no per-project "viewer": restricted
  projects are not enforced against it (see [users.md](users.md)). The root-bearer, proxy-bearer and
  database-key checks are tried in that order, and the first equal secret wins. A proxy asserting the exact
  root pair is root-level; its asserted issuer/subject is also what ownership keys use.

### Identity keys

The server reduces an identity to one key for attribution and ownership:

- If both an issuer and a subject are present (after trimming), the key is the
  pair. The key text is `oidc:<N>:<issuer><subject>`, where `N` is the issuer's length **in bytes** in
  decimal (see [users.md](users.md#identity-keys-and-ownership) for the per-operator directory name).
- Otherwise, if a username is present, the key is the username in the user
  namespace (`user:<name>`). `GET /identity` returns this text in its `operator` field.
- Blank values name nobody.

The pair takes precedence over the username, so a username sent alongside a
complete pair does not affect the key. Moving from a username-only assertion to
the pair changes the key once. Adding a username to an existing pair does not
change it.

## Relation to local accounts

- There is no link between an OIDC identity and a row in the users table. That
  table has no issuer or subject columns. The server does not create or look up
  a local account when it sees an OIDC identity. The identity key is used as the
  owner or attribution value directly.
- Local accounts are used by API credentials and by password login (see
  [users.md](users.md#human-login-password--web-session)). They are not used by SSO.
- A username-only proxy assertion uses the same user namespace as local usernames.
  The server does not check that the name exists locally. For example, a proxy
  asserting `alice` without an issuer and subject is attributed the same way as
  local user `alice`. Operators who need separation should always assert the
  issuer and subject pair.

## Sessions

- **Client.** The stored token entry is the only client-side session state. It
  lasts until logout, until the IdP rejects the refresh token, or until the
  refresh token is replaced. How long the IdP keeps a refresh token valid is set
  by the IdP, not by ai-memory.
- **Server.** SSO adds no server-side session. Each request carries its own
  bearer or proxy assertion. Human browser sessions are a separate mechanism
  based on password login (see [security.md](security.md)).
- **IdP session identifiers.** ai-memory does not read the IdP's session claim.
  The coding-agent session that ai-memory tracks for per-session isolation is
  a different identifier. It arrives in hook payloads or in the
  `X-Memory-Actor-Session-Id` header from a proxy. See [auto-scope.md](auto-scope.md).

## Failure behavior

| Situation | Result |
|---|---|
| Discovery returns a non-success status | Login fails. Nothing is stored. |
| Device authorization returns a non-success status | Login fails. Nothing is stored. |
| User denies, or the device code expires, or the time limit is reached | Login fails with a specific message. Nothing is stored. |
| Token response has no refresh token | Login fails. Nothing is stored. |
| Refresh is rejected by the IdP | The stale access token is used. The server will probably return `401`. |
| Refreshed token cannot be saved | The old access token is used for that request. |
| No stored token can be read (missing or unreadable file) | The request carries no OIDC bearer. |
| Raw OIDC access token sent to a secured server | `401` with a Bearer challenge. |
| Proxy bearer with a repeated or comma-folded identity header | `400`. The request is refused, not resolved to one value. |
| Proxy bearer with no identity, or a partial OIDC pair | `400`. |
| Proxy bearer that does not match the configured value | Falls through to the next check, normally `401`. |

## Deployment shape for SSO-required environments

1. Configure the IdP for the device authorization grant on a public client.
   Allow the `offline_access` scope, so that refresh tokens are issued. Without a
   refresh token, login fails.
2. Have each developer run `ai-memory auth login oidc-device` once, with the
   issuer and client id.
3. For the developer's hooks and CLI to reach a server that requires
   authentication, put a gateway in front of the server. The gateway validates
   the developer's OIDC token, then forwards requests with the proxy bearer and
   the `X-Memory-Actor-*` headers (see [Trusted proxy identity](#trusted-proxy-identity)).
   The gateway also terminates TLS, as described in
   [https-via-proxy.md](https-via-proxy.md).
4. Set `[auth].root_issuer` and `[auth].root_subject` only if the root operator
   should act through the proxy. Keep the root bearer for direct administration.

## Limits

- No signature, issuer, audience, or expiry validation of tokens on the server.
- No IdP key retrieval, token revocation check, or group or role mapping.
- No direct mapping from an OIDC identity to a local account.
- Logout does not revoke tokens at the IdP.
- Only the device authorization grant is covered here. The other provider logins
  in the same command use different flows and are outside this page.
