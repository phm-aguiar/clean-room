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
| Legacy token holder | Bearer equal to a deprecated compatibility token | User | See [Deprecated compatibility token](#deprecated-compatibility-token). |
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
| Legacy token state | Active or expired. Only for deprecated compatibility tokens. |

The account listing never shows password hashes, session secrets, or key secrets and hashes.

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
  revealed.

## Credentials

Each credential class has a format, a storage form, and a level it grants. Prefixes identify the class of a
credential. They do not grant privileges by themselves: the level comes from the class and the owning account.

| Credential | Format | Stored as | Authenticates as | Created by |
|---|---|---|---|---|
| Root bearer | Any non-empty string. The generator yields 64 hexadecimal characters by default | Plain value in configuration. Never in the database | Root | Operator (`generate-auth-token`) |
| Trusted proxy bearer | Any non-empty string that differs from the root bearer | Configuration | Proxy rung: User, or Root for the exact root OIDC pair | Operator |
| Native API key | `aim_` followed by 43 URL-safe characters (32 random bytes) | Digest of the secret, a colon, and the server pepper; plus the last four characters as a non-secret preview | User, attributed to the owning account | `api-key add` or the admin API |
| Compatibility token (deprecated) | 43 URL-safe characters, no prefix | Same digest form, on a reserved key record labelled `legacy-user-token` | User | `user add` (deprecated) |
| Human password | 12 to 1024 bytes | Argon2id PHC string on the account, with default parameters and a per-password salt | Nothing directly. It is exchanged for a web session | `user add-human`, `user reset-password`, password change |
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

### Create a token-only account (deprecated)

`ai-memory user add` (or `POST /admin/users` without a role) creates an account with a compatibility token. The
token is shown once. The account has no password, so it can use the API but cannot log in to the console until
a password is set with `user reset-password`. This path needs the pepper. New automation should use native keys.

### Status values

`ai-memory user list` shows one status per account. The first matching row wins:

| Status | Condition | Meaning |
|---|---|---|
| `disabled` | `disabled_at` is set | Human login is off. Sessions were ended. Native keys still work. |
| `must-change` | Has a password and must change it | Web use is blocked until the password is changed. |
| `active` | Has a password | Normal human login. |
| `expired` | No password and the compatibility token is expired | The token no longer authenticates. |
| `active` (no password) | No password and an active compatibility token | API-only account. |

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

Startup refuses to run in human mode when there is no recoverable root and no recovery token. The operator
runbook is not part of the current scope.

### Change name, email or role

`ai-memory user patch <name>` changes `--name`, `--email` and `--role`. An empty value clears name or email.
Demoting the last recoverable root is refused. Email must stay unique.

### Deprecated compatibility token

These commands manage the compatibility token of a token-only account. They are kept for existing scripts and
are planned for removal in 2.0.

- `user expire` stops the token immediately. Repeating it is harmless.
- `user revive` makes the **same** token valid again. Nothing is rotated.
- `user rotate-token` issues a new token, shown once. The old one returns 401 at once. It also reactivates an
  expired token.

Native keys replace these commands. Each compatibility token is mirrored onto a reserved key record, so
existing clients keep working through the migration.

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
- **List:** `ai-memory api-key list` shows id, label, status (`active` or `revoked`) and preview. The preview is
  the last four characters and cannot be used to authenticate. `--json` prints full records.
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
  is the username. Blank values name nobody. The two forms are stored as `oidc:` followed by the issuer length,
  the issuer and the subject, and as `user:` followed by the username. The namespaces never collide.
- The per-operator directory name is derived from the key. Short lowercase ASCII usernames are readable (for
  example `u-alice`). Mixed-case, trailing-period and other path-unsafe usernames get a deterministic `uh-` name.
  Every issuer/subject pair gets a deterministic `o-` name.

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
- New projects can start restricted with `[auth].new_projects_restricted` (default off). The creator then gets a
  `write` grant.
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

- Only root can change modes and grants. Every change is written to the audit log.
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
- Each UTC day stores at most 128 distinct labels. Extra labels fold into `other`.
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
  before the first session.
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
  - a configured secret matches an existing native key.
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
