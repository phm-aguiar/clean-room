# Users, identity and attribution

> **Status:** current contract. Human password login, web sessions and native `aim_` API keys are the
> account model. The machine root bearer and trusted proxy identity are summarised here and detailed in
> [security.md](security.md) and [sso.md](sso.md).

ai-memory keeps one shared wiki per server. Accounts answer two questions: *who is making this request*
and *who wrote or owns this data*. They do not decide which pages a request may read on their own. Access
can be restricted per project (opt-in, see [Project access](#project-access)); otherwise every authenticated
caller can read every open project on the server.

If you run ai-memory alone, you can skip this page. A single-operator install with no accounts keeps its
historical behaviour.

## When multi-user mode applies

The deployment *distinguishes operators* when either of these is true:

- at least one account exists, or
- a trusted proxy identity bearer (`[auth].actor_proxy_bearer_token`) is configured.

Every gate that depends on this (admin routes, handoff and session ownership, and proposal ownership) uses
the same test, evaluated on each request. Per-operator memory slots are a separate switch (see
[Per-operator memory slots](#per-operator-memory-slots)). Creating the first account closes the
bootstrap admin access without a restart.

Enable multi-user mode when:

- more than one person shares one server and you want writes traced to people;
- you want the audit trail or admission webhooks to see who acted.

Do not treat accounts as access control. They attribute writes. Project access rules are the only gate on
reads, and they are opt-in.

## Principals and levels

| Principal | How it is recognised | Auth level | Notes |
|---|---|---|---|
| Anonymous | No authority configured (no root bearer, no accounts, no human login) | Anonymous | The pre-multi-user default. |
| Machine root | Bearer equal to `[auth].bearer_token` | Root | No account row. Writes are named after `[auth].root_username` when it is set. |
| Root web session | Session of an enabled account with role `root` and no pending password change | Root | Browser console operator. |
| User web session | Session of an enabled account with role `user` and no pending password change | User | |
| Native key holder | Bearer equal to an active `aim_` key | User | Always User, even when the owning account has role `root`. |
| Legacy token holder | Bearer equal to a key row labelled `legacy-user-token`, copied from a 1.x compatibility token by the schema upgrade | User | Authenticates like any native key (looked up by digest). Cannot be created any more; see [Removed 1.x token commands](#removed-1x-token-commands). |
| Proxy-asserted identity | Proxy bearer plus identity headers | Root only for the exact root OIDC pair; otherwise User | No account row is created or read. |

Each account has one of two roles, `root` or `user`. The role sets the level of **web sessions only**. It
never changes what a native key can do, and it never affects the machine root bearer. Root is the operator
role for the console and for `/admin/*`.

## Capabilities by level

| Operation | Root | User | Anonymous |
|---|---|---|---|
| Normal read and write APIs (MCP tools, hook ingestion, page APIs, web reads) | Yes | Yes, subject to [project access](#project-access) | Yes, only when no authority is configured |
| Operational `/admin/*` routes, including read-only status, search and read-page helpers | Yes | 403 when operators are distinguished; no restriction otherwise | 401 when operators are distinguished; no restriction otherwise |
| Account and credential management (`/admin/users*`, `/admin/human-users`, `/admin/api-credentials*`, grants, project access mode) | Yes | 403 | 401. Root-only **always**, including single-operator installs |
| Cross-owner recovery (`any_owner`, `all_owners`, `finalize-session --all-owners`) | Yes | No | No, when operators are distinguished |
| Skip admission webhooks (`X-Memory-Skip-Admission-Chain`) | Yes | Header ignored | Yes |

Root web sessions also get `can_manage_api_keys` from `GET /auth/me` when the account is enabled and has no
pending password change.

Once operators are distinguished, `/admin/*` is root-only as a whole. In a single-operator install with
the root bearer configured, the root bearer is the only way into `/admin/*`.

## Accounts

### Stored fields

| Field | Meaning |
|---|---|
| `username` | Unique identifier. Required. Case-sensitive. Used as the attribution key. |
| `name` | Optional display name, for "last edited by" style UIs. |
| `email` | Optional. Stored lowercased and unique regardless of case. |
| `role` | `root` or `user`. |
| `created_at`, `last_seen_at` | Creation time, and the last time a native key or web session was used. Not updated for the machine root bearer or for proxy identities, which have no account row. |
| `has_password` | Whether human login is possible at all. |
| `must_change_password` | A password change is required before dashboard or API use. |
| `disabled_at` | Set when human login is disabled. |
| `must_change_password`, `disabled_at`, `has_password` | See the rows above. |

The account listing never shows password hashes, session secrets, or key secrets and hashes. (The 1.x compatibility-token columns no longer exist; schema version 79 dropped them.)

### Field rules

- **Username:** trimmed; 1 to 64 characters (counted as characters, not bytes). No whitespace, no control
  characters, and none of `/ \ : ; , " ' \``. Letters from any script, digits, `.`, `-`, `_` and `@` are
  allowed, so `alice@home` works.
- **Name:** trimmed. An empty value is stored as no name. Internal spaces are allowed.
- **Email:** trimmed and lowercased. An empty value is stored as no email. At most 254 characters. Exactly
  one `@`, with non-empty local and domain parts. No whitespace or control characters. A domain without a
  dot is accepted (intranet addresses). This is a sanity check, not RFC validation. Unique case-insensitively.
- **Password:** 12 to 1024 UTF-8 bytes. Must not start with `ams_`, `aim_` or `amk_`. Must not equal the
  username, the root bearer, the proxy bearer, or the recovery token. Must not match an existing native key
  or compatibility token. All policy failures return the same generic message, so the rule that fired is not
  revealed. The inner message is `does not meet policy`; the error text built around it is
  `invalid password: does not meet policy`. Admin routes never accept a password chosen by the operator: password
  reset and human-user creation mint a random temporary password (retried up to 8 times if it collides with a
  reserved secret or an existing key hash), so on those routes a policy failure would be an internal 400
  `{"error": ...}` and is not reachable in practice. The policy text reaches callers through the startup check of
  the initial root password (`initial root password does not meet policy`) and the self-service password change.
- A hook event whose session id already belongs to another owner or agent is **dropped** (terminal
  acknowledgement, outcome `dropped_collision`; a warning in the server log names the rule, never the row); it
  is not an error to the sender and is not retried.

## Credentials

Each credential class has a format, a storage form, and a level it grants. Prefixes identify the class of a
credential. They do not grant privileges by themselves: the level comes from the class and the owning account.

| Credential | Format | Stored as | Authenticates as | Created by |
|---|---|---|---|---|
| Root bearer | Any non-empty string. The generator yields 64 hexadecimal characters by default | Plain value in configuration. Never in the database | Root | Operator (`generate-auth-token`) |
| Trusted proxy bearer | Any non-empty string that differs from the root bearer | Configuration | Proxy rung: User, or Root for the exact root OIDC pair | Operator |
| Native API key | `aim_` followed by 43 URL-safe characters (32 random bytes) | Digest of the secret, a colon, and the server pepper; plus the last four characters as a non-secret preview | User, attributed to the owning account | `api-key add` or the admin API |
| Migrated 1.x compatibility token | 43 URL-safe characters, no prefix | Same digest form, on a key record labelled `legacy-user-token`; an expired one was copied as already revoked | User | Nothing creates new ones (the 1.x commands were removed) |
| Human password | 12 to 1024 bytes | Argon2id PHC string on the account (algorithm Argon2id, version 0x13, memory 19 456 KiB, 2 passes, 1 lane, 16-byte random salt, standard PHC encoding) | Nothing directly. It is exchanged for a web session | `user add-human`, `user reset-password`, password change |
| Temporary password | 32 URL-safe characters, shown once | Only as the password hash | Login only; must be changed at next login | Admin API or CLI |
| Web session secret | `ams_` followed by 43 URL-safe characters, in an HttpOnly cookie | SHA-256, without pepper | Root or User, from the account role | Login, password change |
| CSRF value | 32 URL-safe characters, in a readable cookie | SHA-256, without pepper | Checked on unsafe methods of cookie sessions | Login, password change |
| Recovery token | Configured; at least 32 characters; no reserved prefix | SHA-256, held in process memory from configuration. Never written to the database | Recovery endpoint only | Operator |
| External consumer keys | `amk_` prefix | Not stored or checked by this server | Not accepted as native credentials. Passwords may not start with this prefix | Issued and checked outside this document |

Notes:

- The `aim_` prefix marks native keys only. Other classes exist (the root bearer and the proxy bearer have no
  prefix, and the web session secret uses `ams_`). Native keys always resolve to User, whatever the owner's role.
- Pepper: `ai-memory init` generates one (32 random bytes, 64 hexadecimal characters). It is needed for
  native keys and compatibility tokens only. Do not change it after the first native key is issued. Details
  are in [security.md](security.md).
- The root bearer and the proxy bearer are compared in constant time. Native keys and web session secrets are
  found by a lookup on their digest, not by a constant-time comparison. The digest is what the server holds,
  so a copied database alone does not contain usable secrets.

## Account lifecycle

### Create a person

```console
$ AI_MEMORY_AUTH_TOKEN=<root-token> ai-memory user add-human --username alice --email alice@home --name "Alice Smith"
```

- Requires the root bearer.
- Role defaults to `user`. Pass `--role root` to create a second recoverable root.
- A temporary password is generated and returned **once**. Human-readable details go to stderr and the bare
  password goes to stdout, so it can be piped. The account must change it at next login.
- **No API key is created.** Use `api-key add` for machine access.
- Admin API: `POST /admin/human-users`.

### Bootstrap the first root

On a fresh install, if `AI_MEMORY_AUTH__INITIAL_ROOT_PASSWORD` is set and bootstrap has not completed, startup
creates or promotes the account named by `[auth].root_username` (default `root`) with that password. The account
is flagged to change the password at first login.

- The variable is read only until bootstrap completes. After that it is ignored (a warning is logged). Unset it.
- The password must pass the password rules and must not collide with an existing key. Otherwise startup fails.

### Removed 1.x token commands

`ai-memory user add`, `user expire`, `user revive` and `user rotate-token`, and the routes `POST /admin/users`,
`POST /admin/users/{username}/expire`, `…/revive` and `…/rotate-token`, **no longer exist** (removed in 3.0). A call
returns 404 for any caller (405 for `POST /admin/users`, because `GET /admin/users` still owns the path), after the
usual authentication and root checks (anonymous gets 401, and non-root callers get 403 on `/admin/users`). Accounts that exist
only because of a 1.x token keep working as API-only accounts through their migrated `legacy-user-token` key; manage them with
`api-key list`, `rotate` and `revoke`, and give them a password with `user reset-password` if they need console login.
`api-key rotate` on such a key issues an `aim_` secret.

### Status values

`ai-memory user list` (columns `USERNAME`, `NAME`, `ROLE`, `STATUS`; widths grow to the longest value, minimum 8, 4 and 4; `-` for a missing name; `(no registered users)` when empty) shows one status per account. The first matching row wins:

| Status | Condition | Meaning |
|---|---|---|
| `disabled` | `disabled_at` is set | Human login is off. Sessions were ended. Native keys still work. |
| `must-change` | Has a password and must change it | Web use is blocked until the password is changed. |
| `active` | Anything else (including an account with no password) | Normal. |

### Must-change

- Set by account creation, admin password reset, and bootstrap. Cleared by a successful password change.
  Recovery does not set it.
- While it is set, the session can still read `/auth/me` and change the password. `/admin/*` and `/api/v1`
  return 403 `password change required`, and the capability flags in `/auth/me` are false.
- It does not affect native keys or compatibility tokens.

### Disable and enable

- `ai-memory user disable <name>` asks for confirmation. `--yes` skips the prompt. `enable` does not prompt.
- Disabling:
  - stops human login. A disabled account gets the same 401 response as a wrong password, and login is still
    timed the same way;
  - ends all of the account's web sessions immediately;
  - **does not** revoke native keys or the compatibility token. Those keep authenticating as User. To stop
    machine access, revoke the keys (`api-key revoke`) or expire the compatibility token.
- Enabling clears the disabled flag. It does not restore sessions, so the person logs in again.
- The last enabled root account that has a password cannot be disabled or demoted.
- Disabled accounts keep their history. Attribution references continue to resolve.

### Reset a password (admin)

`ai-memory user reset-password <name>` issues a new temporary password (shown once), sets must-change, and
ends all of the account's web sessions. A disabled account stays disabled. A token-only account gains human
login with this command.

### Change your own password

`POST /auth/password` with the current password, the new password and its confirmation.

- Wrong current password: 401. Confirmation mismatch: 400. Policy failure or reserved value: 400.
- On success (204): other sessions of the account are revoked, the current session keeps working with a new
  secret and CSRF value, and must-change is cleared.

### Login

`POST /auth/login` with `username` and `password`. On success it returns the `/auth/me` snapshot and sets the
session cookies. Every credential failure returns 401 `invalid credentials`, including unknown users, disabled
accounts and token-only accounts. Rate limiting returns 429. Login limits and the rest of the browser session
rules are in [security.md](security.md).

Exact order of checks and responses (JSON bodies are `{"error": "<text>"}`):

1. No human-auth runtime: 401 `auth required`.
2. `Content-Type` must be `application/json` or `application/*+json` (case and parameters ignored), otherwise 415 `expected Content-Type: application/json` (this blocks cross-site form posts).
3. The client address is resolved (see "Client address" below). If that address already has 10 failures inside the last 60 seconds, 429 `too many login attempts` — before any password work.
4. Body up to 64 KiB with `username` and `password`, otherwise 400 `invalid body`. The username is trimmed (no other validation).
5. Unknown user, disabled user, or user without a password: a throw-away verification of the typed password against a fixed dummy hash is run (same cost as a real miss), one failure is recorded for the address, and the answer is 401 `invalid credentials`. If the password worker pool is saturated, 429 `kdf saturated` and no failure is recorded.
6. Otherwise the password is verified; a wrong password records one failure and answers 401 `invalid credentials`.
7. A session is created only if the account row still matches what was verified (same password hash, role, must-change flag, not disabled); otherwise 401 `invalid credentials`.
8. Success: 200 with the snapshot `{username, name, role, must_change_password, via: "session", capabilities: {normal_read, normal_write, admin, user_management}}` (no email, no id) and three `Set-Cookie` headers: the session cookie `ai_memory_session=<secret>; HttpOnly; SameSite=Strict; Path=/; Max-Age=<seconds to expiry>[; Secure]`, the CSRF cookie `ai_memory_csrf=<value>; SameSite=Strict; Path=/; Max-Age=…[; Secure]` (readable by scripts), and an expiring `ai_memory_auth=` legacy cookie. Only failures count toward the limit; successes are not recorded.

Password hashing and verification run in a separate pool: at most 2 run at once, at most 32 may wait; more than that fails immediately with 429 `kdf saturated`.

**Client address.** The address used for the limit is the TCP peer, unless the peer is inside one of the configured `[auth].trusted_proxy_cidrs` (IPv4 or IPv6 CIDR, a bare address meaning /32 or /128; an invalid entry stops startup). Then all `X-Forwarded-For` field lines are read in order and split on commas; if any hop is not an IP address or no header exists, the peer is used; otherwise the **rightmost hop outside the trusted ranges** is used (the leftmost hop if all are trusted). With no peer information the address is 0.0.0.0. The limiter keeps at most 4 096 addresses (when full, expired ones are dropped, then the one whose oldest failure is oldest); an attempt ages out individually when strictly older than 60 seconds.

**Session cookies and CSRF.** A session secret is `ams_` + 43 URL-safe characters; the CSRF value is 32 URL-safe characters (24 random bytes). Only SHA-256 digests are stored. The session lasts exactly 7 days from login (absolute; password change keeps the same expiry), looked up by digest; revoked, expired or disabled-account sessions answer 401 `auth required`. For methods other than GET, HEAD and OPTIONS a cookie session must send the header `X-CSRF-Token` equal to the CSRF cookie (same length, constant-time compare) and whose digest matches the session's; otherwise 403 `csrf required`. A request that presents any Bearer is never treated as a session on `/auth/*` (401 `auth required`). On each authenticated session request the session's last-used time (written at most once a minute) and the user's `last_seen_at` are updated in the background.

**Bearer parsing.** Authorization headers are scanned in order; non-Bearer ones are ignored; the scheme is case-insensitive and may be separated from the token by a space or tab; the token is trimmed; two Bearer headers make the token empty (rejected). A failed machine bearer answers 401 with body `auth required` and `WWW-Authenticate: Bearer realm="ai-memory", error="invalid_token"`. Order of matching: the root bearer (constant-time), then the trusted-proxy bearer (feature 009), then the key digest lookup. With no authority configured at all, a bearer is ignored and the caller is anonymous. A blank configured bearer counts as not configured.

### Recovery (break-glass)

Recovery exists for a lost root password. It needs `AI_MEMORY_AUTH__RECOVERY_TOKEN` (at least 32 characters).

`POST /auth/recovery` with `recovery_token`, `new_password` and `new_password_confirmation`:

- The target is the account named by `[auth].root_username` (default `root`). It is created or promoted to root
  with the new password.
- Its web sessions are revoked, and bootstrap is marked complete.
- **No session is issued.** Log in afterwards. The response is 204.
- The recovered root is not forced to change the password.
- Wrong token, recovery not configured, policy failure, or mismatch all return the same 401
  `invalid credentials`. Recovery failures count against the same per-address login limit.
- The same JSON content-type rule (415), address limit (429 `too many login attempts`), and 64 KiB body limit (400 `invalid body`) as login apply. The token is compared by SHA-256 digest in constant time. Order: token check, then confirmation equality, then password policy (with the root username as the "equals the username" rule) and reserved values, then key collision, then hashing; every refusal runs a dummy verification, records a failure and answers 401. The target is created with name and email from `[auth].root_name` and `root_email` when it did not exist, and, if it existed, name/email are only filled in when those settings are present. Existing target: role becomes `root`, the password is replaced, must-change is cleared, a disabled flag is cleared, sessions revoked, bootstrap marked complete. Success answers 204 and an expiring `ai_memory_auth` cookie.
- Bootstrap (`AI_MEMORY_AUTH__INITIAL_ROOT_PASSWORD`) does the same create-or-promote but sets must-change and, when it targets an existing account, also clears its disabled flag.

### Change-password responses

`POST /auth/password` (session and CSRF required) with `current_password`, `new_password`, `new_password_confirmation`: 400 `password confirmation does not match`; policy failure 400 with the policy text `does not meet policy`; reserved value or collision with an existing key digest 400 `password is reserved`; account/hash gone 401 `auth required`; wrong current password 401 `invalid credentials` (not rate limited); concurrent change of the password, session or disabled flag 409 with the reason. Success is 204 with new session and CSRF cookies on the same absolute expiry.

### Logout

`POST /auth/logout` (session required) revokes the session named by the cookie, always answers 204 and clears the three cookies (`Max-Age=0`).

### `/auth/me`

`GET /auth/me`: with no authority configured it answers `{username: null, name: null, role: null, must_change_password: false, via: "anonymous", capabilities}` (`normal_read`, `normal_write` and `admin` true, `user_management` false, because user management is root-only even with no authority configured); otherwise it needs a session and answers the same snapshot as login. When the password change is pending every capability is false. Every `/auth/me` response also expires the legacy cookie.

Startup refuses to run in human mode when there is no recoverable root and no recovery token. The operator
runbook is not part of the current scope.

### Change name, email or role

`ai-memory user patch <name>` changes `--name`, `--email` and `--role`. An empty value clears name or email.
Demoting the last recoverable root is refused. Email must stay unique.

## Native API keys

Native keys are the machine credential. Create, rotate and revoke them with root access.

- **Issue:** `ai-memory api-key add --username <name> --label <label>`. The secret is printed once on stdout.
  Details go to stderr. The account must exist, and the pepper must be configured (otherwise the admin API
  returns 503).
- **Several keys per account:** allowed. Each key has its own label and is an independent credential. Keys do
  not depend on the account password.
- **Use:** send as a bearer. The request authenticates as User of the owning account, and `last_used_at` is
  updated.
- **Rotate:** `ai-memory api-key rotate <id>` writes a new secret onto the same record. The old secret is rejected
  immediately. If the record was revoked, rotation makes it active again with the new secret. The old secret
  never comes back.
- **Revoke:** `ai-memory api-key revoke <id>` (asks for confirmation unless `--yes`). Idempotent. The record stays
  in the list with status `revoked`.
- **List:** `ai-memory api-key list` shows id, label, status (`active` or `revoked`; an expired key still shows `active`) and preview. The preview is
  `…` (U+2026) plus the last four characters of the secret and cannot be used to authenticate. `--json` prints full records
  (`id`, `user_id`, `label`, `preview`, `created_at`, `last_used_at`, `expires_at`, `revoked_at`, with absent values omitted), newest first.
- **Wire and terminal behavior:** `api-key add` and `rotate` print the secret on stdout and a confirmation (`✓ created|rotated API key '<id>' (<label>)` plus a "will NOT be shown again" note) on stderr; `rotate` and `revoke` ask `(y/N)` unless `--yes` (end of input aborts). Admin routes answer 200 with `{credential, token}` (create, rotate) or `{credential}` (revoke); an unknown id on rotate is 404 `no such credential`, a malformed id is 400. Secret = `aim_` + 43 URL-safe base64 characters (32 random bytes, no padding). Stored digest = SHA-256 of `secret ":" pepper`. `last_used_at` and the owner's `last_seen_at` are updated in the background on every authenticated request (no throttle).
- **Expiry:** a record can carry an optional expiry, and the lookup honours it. No current command sets it.
- **Owner state:** disabling the owner, resetting its password, or changing its role does not change its keys.
  Revoke them separately.

## Human login (password + web session)

Passwords are for people using the browser console. They never authenticate as a bearer. The full session and
browser rules are in [security.md](security.md). The user-facing points:

- Logging in sets an HttpOnly session cookie (`SameSite=Strict`, `Path=/`, `Secure` when `[auth].secure_cookie`
  is on) and a readable CSRF cookie. Both expire with the session.
- Sessions last 7 days from login, with no idle timeout. Last-used time is updated at most once a minute.
- Unsafe methods on cookie sessions (anything except GET, HEAD and OPTIONS) must send the CSRF value in the
  request header. Machine bearers skip this check.
- Session secrets never authenticate `/mcp`, `/hook`, `/handoff` or `/workstream/*`. Passwords never authenticate
  as a bearer.
- Sessions are revoked by logout, by a password change (other sessions), by an admin password reset, by disable,
  and by recovery.
- `/admin/*` and `/api/v1/*` accept a machine bearer **or** a web session. The custom web app shell is public, and
  its API calls are authenticated. The login and change-password pages are public.
- **Transition from the legacy browser transport:** until a password exists or bootstrap completes, GET-only
  browser routes accept the root bearer through HTTP Basic or a legacy cookie. Human activation turns that off
  immediately, without a restart. Machine routes stay bearer-only.

## Trusted proxy identity

A deployment that puts SSO in front of the server authenticates the person at the proxy and then calls the
server with the proxy bearer. The person is described in `X-Memory-Actor-*` headers. The full rules are in
[sso.md](sso.md). The account-facing points are:

- Identity headers are honoured only with the proxy bearer. The root bearer and account bearers ignore them.
- The proxy must **replace** client-supplied identity headers. A repeated header, a comma-folded value, an
  identity with no user and no issuer/subject pair, or a partial issuer/subject pair returns 400.
- An asserted identity is User by default. It is Root only when the issuer and subject exactly match
  `[auth].root_issuer` and `[auth].root_subject`. A username never grants root.
- A proxy identity has **no account row**. Nothing is created for it, and no account is looked up. It is
  attributed through its identity key (see [Identity keys](#identity-keys-and-ownership)). Because there is no
  account id, pages and audit rows written by proxy identities do not record an account id (see
  [Attribution](#attribution)).
- Configure the proxy bearer only when the server is reachable only through that proxy.

## Identity keys and ownership

Ownership and attribution use one qualified key per identity, never a bare string:

- If an issuer and a subject are both present, the key is that pair. Otherwise, if a username is present, the key
  is the username. Blank values (empty after trimming) name nobody. The two forms are stored exactly as
  `oidc:<N>:<issuer><subject>` where `N` is the **byte** length of the issuer in decimal followed by a colon, and as `user:<username>`.
  The namespaces never collide. Parsing a stored key back reverses this and rejects blank parts.
- The per-operator directory name (path segment) is derived from the key: `u-<username>` when the username is non-empty, at most 64 bytes, has no trailing `.`
  and contains only `a-z`, `0-9`, `.`, `_` and `-`; otherwise `uh-<hex>` for a username, and `o-<hex>` for every issuer/subject pair, where `<hex>` is the
  32-character lowercase hexadecimal form (no hyphens) of the version-5 UUID of the stored key text in the standard URL namespace
  (`6ba7b811-9dad-11d1-80b4-00c04fd430c8`).

### Owner stamping

Handoffs and sessions carry an owner, which is the identity key of the request that created them.

- Owners are stamped only where the deployment distinguishes operators. A single-operator install stores no
  owner, even when the root bearer has a `root_username`. Otherwise the stdio transport and local CLI would stop
  seeing data written over HTTP.
- An owner of `NULL` means **shared with the project**. Rows written before ownership existed, and rows written
  without an authenticated identity, stay visible to everyone.
- Reads select the caller's own rows plus shared rows. A caller without an identity sees only shared rows. Root
  recovery paths can see all owners.
- Reads are not gated by the deployment mode. A row stamped while operators were distinguished stays readable
  by its owner afterwards.

Handoffs and sessions:

- `memory_handoff_begin` with `shared: true` publishes a handoff to everyone in the project.
- `memory_handoff_list`, `memory_handoff_accept` and `memory_handoff_cancel` take `any_owner: true` to act on
  another operator's handoff. This needs admin authority when operators are distinguished.
- `ai-memory finalize-session --all-owners` (`GET /admin/open-sessions?all_owners=true`) reaches other operators'
  open sessions. `--session-id <uuid>` narrows the lookup to one session. It cannot be combined with `--all`.
- `GET /admin/sessions/by-agent` counts sessions per agent CLI in one scope (`workspace` and `project` are
  required, `since_days=N` is optional, and zero means all history). It returns the caller's sessions plus
  unowned ones, or every operator's with `all_owners=true`. Results are `{"by_agent":[{"agent":"codex","sessions":3}]}`,
  ordered by count and then agent name. An unknown scope is an error and nothing is created.
- The read-only handoff listing for a project returns prompt-derived fields (`summary`, `open_questions`,
  `next_steps`) to callers the server can place (their own and shared rows) and to root. Other callers get the
  metadata with `redacted: true`. The default listing is own plus shared. Root can ask for the recovery view with
  `?all_owners=true`. User and anonymous callers get 403 for that view.
- Handoff begin, accept and cancel raise admission operations. A webhook with `failure_policy = "reject"` can refuse
  them. Other webhooks are notified after the operation is durable.
- Session lookups from hooks are scoped to the caller's identity, so one operator's hook does not see another
  operator's open session through the shared lookup. The exact response to a foreign session identifier:
  a confirmar.

## Attribution

Attribution records *who did it*. It never decides *whether they could*.

| Surface | What is recorded | Notes |
|---|---|---|
| Latest page version | Author account id | Set only when the writer is a native key holder or a web session. Otherwise empty. |
| Page frontmatter `last_modified_by` | `username`, plus `name` and `email` when set | Written only when the actor has a username. An OIDC-only proxy assertion does not get this block. |
| Audit log rows | Account id on every mutating operation | The reference is cleared if an account row is ever removed. No command removes accounts today. Grant and project access changes are recorded (see [Project access](#project-access)). |
| Page API responses | `author` object: `username`, and `name` and `email` when set | Shows the account's current values. |
| Single-page ETag | Changes when the author changes | The tag is a digest of the response, so it changes with the author fields. |
| Web UI page view | Author of the page | |
| Admission webhooks | Actor identity in the payload | Same identity as the frontmatter block. |
| Hook installs | `--as-user` label | A label only. See [Hooks and agent wiring](#hooks-and-agent-wiring). |

Which identity writes what:

- Native key holders and web sessions (root or user): the account id and username are recorded everywhere above.
- Machine root bearer: writes are named after `root_username` when it is set. No account id is recorded.
- Proxy-asserted identities: writes carry the identity key, and frontmatter carries the username when the proxy
  asserts one. No account id is recorded.
- Account and credential lifecycle actions (create, disable, reset, key issue and revoke) do not write audit rows
  in the current build.

Reads are never filtered by author. `pages.author_id` is attribution, not a read filter.

## Running for a team

**Knowledge is shared; batons are owned.** A page written in a project is readable by every operator who can
read that project. Handoffs are different: a handoff carries an owner, and only that operator receives it.

**Concurrent edits supersede rather than collide.** Two people editing the same page produce a version chain.
The later write becomes latest and the earlier one stays reachable. There is no merge.

**The "current project" pointer is isolated per caller.** Unscoped calls resolve through a pointer keyed by the
caller's identity and session, so two operators, or one operator with two harnesses, do not overwrite each other.
Check the mode at startup:

```text
active-project isolation mode mode=PerActor session_ttl_secs=3600 max_entries=4096
```

If a shared server reports `Single`, unscoped reads and writes from concurrent sessions share one
last-write-wins slot. See [auto-scope.md](auto-scope.md).

## Project access

Project access is the only read gate in this document. It is **open by default**: every authenticated caller can
read every project on the server.

- **Open** (default): any caller admitted to the server can read and write the project.
- **Restricted** (set per project): only root, the project creator, and holders of a grant can enter. A `read`
  grant admits reads. A `write` grant admits writes too.
- New projects can start restricted with `[auth].new_projects_restricted` (default off; read once at startup, never changes existing projects). The creator
  (a database user who created the project row; root-bearer creations and single-operator installs record none) is admitted by the creator rule: **no grant row is written for the creator**, and the creator is
  admitted only while the project is open or restricted, never to the shared global scope's write gate.
- Reserved shared scopes (the global scope and each workspace's profile scope) stay readable by everyone. Writing
  them needs root or a `write` grant on that scope, whatever its mode.
- The check applies at the entry point of every project-scoped read and write: MCP tools, the page API, the web
  UI, and cross-scope search. Restricted projects are filtered out of cross-scope search for native-key and
  web-session users.
- If the grants cannot be read, the decision degrades to open and logs a warning. It never locks out a project
  silently.

Commands and routes:

```console
$ ai-memory project access --workspace default --project notes --mode restricted
$ ai-memory user grant --user alice --workspace default --project notes --level write
$ ai-memory user grants --user alice
$ ai-memory user revoke --user alice --workspace default --project notes
```

- Only root can change modes and grants. Every grant change is written to the audit log (`grant_access` with detail `{user_id, level, previous}`; `revoke_access` with `{user_id, level}`); a re-grant at the same level or a revoke of nothing writes no row. Access-mode changes are not audited.
- Routes (all 200 on success, 400 for a missing or unknown `level`/`mode`, 404 for an unknown user or project; none creates a project): `POST /admin/users/{username}/grant` `{workspace, project, level}` → `{username, workspace, project, level, changed, previous}` (`changed` false and `previous` equal to the level when nothing changed; `previous` null for a new grant); `POST …/revoke` `{workspace, project}` → `{username, workspace, project, revoked}`; `GET /admin/users/{username}/grants` and `GET /admin/projects/grants[?workspace=&project=]` → `{grants: [{username, workspace, project, level}]}` (give both names or neither, else 400); `POST /admin/projects/access` `{workspace, project, mode}` → `{workspace, project, mode, previous, changed, without_access}` where `without_access` (only when restricting, else empty) lists the usernames that authored pages in the project but are not root, not disabled, not the creator and hold no grant. The mode of `_global` and of any profile project (`_profile`, `_profile.*`) cannot be changed (400).
- Grants are per project, not per page. Pages are shared inside a project, and authorship never filters reads.
- Current limit: the check is applied to native-key and web-session users. Proxy-asserted identities are not
  checked against restricted projects in the current build. See [Limitations](#limitations).

The design and the decision table are in [design-per-project-authz.md](design-per-project-authz.md). The boundary
inventory is in [security-boundaries.md](security-boundaries.md).

## Per-operator memory slots

Slots are wiki pages under `_slots/`. The `[slots] per_user` switch (default **off**) scopes them per operator.
With it off, every slot goes into every session brief, as before the feature existed. With it on:

- Session briefs and consolidation prompts show shared slots plus the viewer's own, including the list of
  recently touched pages. Another operator's slot path and title stay out of the brief.
- A slot at `_slots/<segment>/current-focus.md` is injected only into the operator whose segment matches. The
  segment is the identity key's per-operator directory name (see above).
- Writing the shared slot through the page API is redirected to the caller's own namespace. The response reports
  the path actually written. A write into another operator's namespace is refused. Root may still curate any
  namespace.
- The consolidation run writes its shared slot into the session operator's namespace. A path the model aims at a
  foreign namespace is skipped, not re-homed.
- Scope is injection, not access. Exact reads and searches still see every page in the project.
- Slots written before this feature are shared.

Turning the switch off makes personal slots visible to everyone again. Nothing is stranded. Upgrading a shared
server with the switch on needs care. Directories named `u-…` may have been created under a different case rule
on case-insensitive filesystems. Review them and re-home confirmed content into the owning operator's namespace.
Keep the old pages until ownership is clear. Do not infer ownership from filename casing alone.

One deliberate gap: the bootstrap operation writes pages at paths chosen from the ingested repository content, with
no operator to attribute them to. It is admin-only. Review the bootstrap section of [api-reference.md](api-reference.md)
for the paths it can write.

## Per-operator state

Beyond attribution, some engine state records the operator. Absence means shared, so a single-operator server keeps
its historical behaviour.

- **Auto-improvement proposals.** A proposal staged through a request records the identity key of the operator who
  staged it. The one-pending-proposal-per-page rule applies per operator when operators are distinguished, so
  operators do not block each other. Scheduled runs have no caller and stage unattributed. A proposal that collides
  with a pending one is skipped alone, and the staging responses report the skip with the target path and reason.
- **Page reinforcement.** The first reinforced read by each identified operator is recorded per page, next to the
  shared access counter. `[decay] breadth_weight` (default `0.0`) lets a page read by many different operators
  outrank one read repeatedly by a single operator. At the default, and for pages with fewer than two distinct
  readers, retention scores are unchanged.

## MCP client activity

`GET /admin/activity/by-client` reports server-wide MCP tool calls by client. It is root-only when operators are
distinguished.

- Clients are named by the sanitised MCP client name. Stateless requests fall back to the authenticated proxy's
  agent label, then to `unknown`.
- Results look like `{"by_client":[{"client":"claude-code","reads":12,"writes":3}]}`, ordered by total calls and
  then client name.
- `since_days=N` includes every UTC day that intersects the lookback. Zero or omitted means all history.
- Counts flush on a one-minute interval even when no request follows. A failed batch is retried once. A process
  exit can lose the current interval.
- Each UTC day stores at most 128 distinct labels. Extra labels fold into `other`. A client label is trimmed, bidirectional control characters are dropped, other whitespace runs collapse to single spaces, other control characters are dropped, and the result is cut to 64 characters; empty means unknown.
- A call counts as a **read** only for these tools: `memory_query`, `memory_read_page`, `memory_read_session_observations`, `memory_recent`, `memory_briefing`, `memory_explore`, `memory_status`, `memory_handoff_list`, `memory_message_list`, `memory_install_self_routing`. Every other tool, including any added later, counts as a write.
- The endpoint takes no workspace or project, because many MCP-only clients do not send reliable per-call scope.

## Hooks and agent wiring

After `ai-memory api-key add` prints a native key, wire the person's agent install to it:

```console
$ ai-memory api-key add --username alice --label claude-hooks
$ ai-memory install-hooks --apply --agent claude-code --as-user alice --auth-token aim_<secret>
```

- `--auth-token` is the credential the hooks will send. Without it, the hooks use the configured root bearer and
  act as root.
- `--as-user` is a label for your records. It requires `--auth-token`, and the server is not asked to verify
  that the token belongs to that user. The command prints a confirmation line so you can check the identity
  before the first session: `[ai-memory] hooks installing for user: <name>` on standard error. The value is
  trimmed; an empty or whitespace-only `--as-user` counts as not set. Without a usable `--auth-token` (also
  trimmed) the command fails: `--as-user '<name>' requires --auth-token (issue one with `ai-memory api-key add
  --username <name> --label <label>`)`. The check runs against the token as given, including when it is then
  stored in the data directory instead of the agent configuration. Nothing contacts the server.
- With `--apply`, the token is stored in two files in the data directory: one holding the bare token, and one
  holding a complete authorization header. Both are owner-only, and the data directory itself is owner-only.
  These permissions apply on Unix. On Windows, the data directory's folder permissions are the protection. The token
  is kept out of the agent configuration and out of command lines.
- An explicit `--auth-token` on a hook command takes precedence over the stored file. Generated hook scripts read
  `AI_MEMORY_AUTH_TOKEN` from their environment. Which harnesses honour that variable: a confirmar per harness.
- Run `install-hooks --apply` again to move an existing install onto the stored form.

### Rotation and spooled hook events

A lifecycle hook that cannot reach the server writes the event to the local spool together with the bearer it held
at capture time. Rotating a token therefore leaves spooled events with the old bearer, and the server rejects them
with 401 on the next drain.

- The drain recovers them. Re-run `install-hooks --apply` with the new token. The next drain retries each 401 entry
  once with the current bearer, which the spawning hook passes to the drain process.
- The retry only applies to static bearers. Refreshed OIDC tokens are re-resolved on every pass anyway. It only runs
  when the current bearer differs from the one just rejected.
- Until the hooks hold the new token, the events stay queued and age out on the normal retry budget. They do not
  block the rest of the spool.

## Local root bearer token file (fresh installs and `auth enable`)

The root bearer can live in a one-line, owner-only file instead of in the configuration or the environment. The
file keeps the secret out of every harness configuration and out of every command line.

- **Setting.** `[auth].bearer_token_file`. A relative path resolves against the data directory. `AI_MEMORY_AUTH_TOKEN`
  wins over it, as it wins over the inline `[auth].bearer_token`. Setting both `[auth].bearer_token` and
  `[auth].bearer_token_file` in the configuration is refused at load:
  `[auth].bearer_token and [auth].bearer_token_file are both set; keep only one`. The file content is trimmed; a
  missing or blank file leaves no bearer, and the server then **refuses to start** instead of running open.
- **Implicit discovery.** A client command that loads **no configuration file** (the configuration lives elsewhere
  and was not passed) still uses `<data>/auth-tokens/default` as its bearer, as if it were file-sourced, when all of
  these hold: no bearer is supplied any other way; no configuration file exists where the load looked; the server
  URL is loopback; no server profile named `default` is registered.
- **`init` on a fresh install.** "Fresh" means no configuration file and no database file yet. `init` then mints a
  root bearer (32 random bytes, 64 hexadecimal characters) at `<data>/auth-tokens/default` (the directory
  owner-only, the file `0600` where modes exist; on Windows the data directory's folder permissions are the
  protection), gives the hooks the same token through the hook credential files `auth-token` and `auth-header`
  (unless another credential is already stored there), and records `bearer_token_file = "auth-tokens/default"` in
  the new configuration. It prints `Generated a root bearer token in <path> (owner-only). The server requires it on
  every route but /healthz; hooks, the CLI and `ai-memory mcp-bridge` read it from there.` An existing token file is
  reused, never rotated. A `--force` rewrite of a configuration that already names a token file keeps naming it (and
  mints the file only if it is missing or blank). Any install that already has a configuration or a database, or has
  a bearer supplied another way, records nothing and keeps its previous behavior (loopback servers without
  authentication).
- **`auth enable`.** The opt-in for installs that predate that default; idempotent. Option `--no-mcp` leaves harness
  registrations alone. Steps: (1) if a bearer is already supplied inline (`AI_MEMORY_AUTH_TOKEN` or
  `[auth].bearer_token`) it prints `auth: already required by AI_MEMORY_AUTH_TOKEN or [auth].bearer_token; nothing
  changed.` and stops; (2) refuses when a server profile named `default` is registered (its token file would be the
  same file) and tells the operator to re-register it under another name; (3) reuses or mints the token file;
  (4) records `bearer_token_file` in the configuration, keeping comments and other keys (creating an owner-only file
  when none exists; an existing file keeps its permission bits); (5) seeds the hook credential files unless a
  different credential is stored; (6) re-registers, through `install-mcp`, every client whose `ai-memory` entry
  already points at this server, keeping a pinned schema dialect. Output lines: `auth: bearer token generated|kept at
  <path> (owner-only)`, `config: recorded [auth].bearer_token_file in <path>` or `config: already names the token
  file: <path>`, one `hooks:` line (`given the token through the hook credential files`, `already use the token`, or
  a note that another credential was kept and `install-hooks --apply` switches it), one `mcp <client>:` line per
  client (`re-registered in <path> so it reads the token without holding it`; `targets <url>, another server; left
  unchanged`; `needs a manual change: <reason>`), and finally `Restart the ai-memory server for it to require the
  token.` One failing client never stops the others. Pi (no MCP file) and DeepSeek Harness (print only) are skipped.
- **How installers treat a file-sourced token.** `install-mcp` never writes a token read from the token file into a
  harness configuration (only a token supplied inline is rendered). Instead: Claude Code gets the session-aware
  bridge; Codex, Prime Agent and Droid reference the `AI_MEMORY_AUTH_TOKEN` environment variable by name; Pi and Muse
  are unchanged (Pi's generated extension reads the hook credential files; Muse's entry is always the bridge); every
  other client gets a stdio entry that runs `mcp-bridge --no-session-id --server-url <url>` in that client's own
  documented shape, and the bridge reads the token file at launch. ZCode keeps only HTTP entries, so it is refused
  with an explanation (add the header by hand or pass the token explicitly); the refusal text names the token file.
- **Posture lines.** `status` and `doctor` report the authentication posture: `bearer token`; `none (loopback) — fix:
  `ai-memory auth enable``; or `none — set AI_MEMORY_AUTH_TOKEN on the server` (open on a non-loopback address).

## Migration and backward compatibility

- **Machine bearer only.** No action is needed. The root bearer keeps working as before. Set up human login only
  when you want passwords.
- **Enable accounts on an existing single-user install.**
  1. Back up the configuration file.
  2. Generate a pepper with `ai-memory generate-auth-token 32`. It prints 64 hexadecimal characters, the same shape
     `init` generates.
  3. Add `token_pepper` and, optionally, `root_username`, `root_email` and `root_name` to the `[auth]` block. Keep
     your existing bearer.
  4. Restart `ai-memory serve`.
- **Startup refusals.** The server refuses to start when:
  - native keys or compatibility tokens exist and the pepper is missing or blank;
  - accounts exist, human login is not active, and the root bearer or the pepper is missing;
  - human login is active with no recoverable root and no recovery token;
  - human login is active on a non-loopback plain HTTP address without `[auth].secure_cookie`;
  - the proxy bearer is set without a root bearer, or equals it, or the issuer and subject are not set together;
  - two of the root bearer, proxy bearer, recovery token and initial root password are equal;
  - a configured secret matches an existing native key;
  - the first bootstrap password fails the password rules or collides with an existing key;
  - `[auth].bearer_token_file` names a missing or empty file;
  - a `[auth].trusted_proxy_cidrs` entry is not a valid address or CIDR;
  - the recovery token is shorter than 32 characters or starts with `ams_`, `aim_` or `amk_`;
  - the server is bound to a non-loopback address with no authentication configured, unless `--allow-insecure-no-auth` is given (inside a container only a warning is printed).
  Human mode is "intended" when bootstrap already completed, or any account has a password, or an initial root password or a recovery token is configured. The messages are free text; the pepper and bearer checks are independent (the message names whichever is missing). The first-account bootstrap password is also rejected if it equals the root bearer, the proxy bearer, the recovery token or the root username.
- **Schema upgrades** add password, session and key storage, and copy existing compatibility tokens onto key
  records. Nothing needs to be re-issued. The deprecated commands keep working until 2.0.

## Limitations

- **Accounts are not a tenancy boundary.** The default is open: any authenticated caller can read every open project
  on the server, including through cross-scope search. If one server would hold work for more than one team, restrict
  each team's projects with grants, or run separate servers behind a reverse proxy. Per-project authorization is
  described above.
- **Restricted projects do not apply to proxy-asserted identities** in the current build. Use separate servers if this
  matters for your deployment.
- **No per-page permissions** inside a project. Pages are shared by design, and authorship is never a read filter.
- **Disabling an account does not revoke its native keys.** Revoke them separately. Disabling only stops human login
  and ends web sessions.
- **There is no account deletion.** Use disable for people who leave. Historical attribution stays intact.
- **Native keys are not passwords.** `user add-human` never issues a key, and `api-key add` never issues a login.
  An account may hold several keys with distinct labels. Its password never works as a bearer.
- **The root bearer is one shared secret.** Change it in configuration and restart. A native key on a root account is
  still User. Root automation should use the root bearer or a root web session.
- **Proxy and root-bearer writes have no account id** on pages or audit rows. Their name is recorded in frontmatter
  when a username is present.
- **Account and key lifecycle actions are not in the audit log** in the current build. Grant and project access
  changes are.
- **OIDC is request authentication, not page authorization.** The server does not validate tokens or map identities
  to local accounts. See [sso.md](sso.md).

## Related documents

- [security.md](security.md): bearer, pepper, password, session and login rules, and deployment checks.
- [sso.md](sso.md): trusted proxy identity and SSO limits.
- [security-boundaries.md](security-boundaries.md): trust boundaries and the adversarial test map.
- [design-per-project-authz.md](design-per-project-authz.md): per-project access design.
- [auto-scope.md](auto-scope.md): active project pointer and session auto-scope.
- [api-reference.md](api-reference.md): route list and payloads.
- [cli.md](cli.md): command reference.
- [design-decisions.md](design-decisions.md): the exclusions section for the first release.
