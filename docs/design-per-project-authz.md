# Design: per-project authorization on multi-user servers

**Status: partially implemented.** This document describes the per-project access design and the behavior of the current build. Sections marked **Implemented** describe behavior that exists today. Sections marked **Proposal** describe intent that is not built. Items marked **a confirmar** were not verified against the source and must be checked before anyone relies on them.

## Implementation status

| Area | Status |
|---|---|
| Per-project access mode and per-user grants (read, write) | Implemented |
| Decision rule for open and restricted projects, and for the shared global scope | Implemented |
| Degrade-to-open on unreadable access data, and fail-closed "strict" resolution for managed-run mutations | Implemented |
| Enforcement on scoped reads and writes | Implemented |
| Filtering of unscoped reads (cross-project query, recent, global web search) before the result limit | Implemented |
| Gates on raw-id entry points (session, run, handoff, messaging) | Implemented, except the items under [Known gaps](#known-gaps-a-confirmar) |
| Management commands and root-only admin routes | Implemented |
| Server setting that makes new projects restricted by default | Implemented; off by default |
| Per-project admin delegation, row-level access control, time-limited grants, groups and roles | Proposal (non-goals for this version) |
| Structural enforcement that makes unguarded lookups private to the module | Not applied (unscoped by-id lookups stay public; see Known gaps) |

## Problem

On a multi-user server (one where database users exist, or a trusted identity proxy is configured), a database-user token attributes writes to that user but does not, by itself, limit which projects the user may read or write. Before per-project access existed, any authenticated non-root caller could resolve any workspace and project and read or write it. Attribution existed; authorization did not.

Administrative routes were already root-only. Handoffs were already isolated per operator. Pages, observations and search were scoped by project only, never by user.

The default posture (loopback, single operator) is unaffected, which is why single-user installs did not hit this gap. It matters for a shared server that hosts several teams.

## Authorization levels (Implemented)

Requests are classified into three levels: **anonymous**, **user** (a database user, or a proxy-asserted identity) and **root** (the configured root token).

Capabilities and how each level is treated:

| Capability | Single-operator server | Multi-user server |
|---|---|---|
| Admin operations | Any level | Root only. Anonymous gets an authentication-required error; user gets a forbidden error. |
| User management | Root only, with authentication required | Same as single-operator |
| Normal read and write | Every level | Every level. Per-project checks are applied separately (see below). |
| Skip the admission chain | Anonymous and root allowed; user refused | Same |

"Multi-user" means the deployment distinguishes operators. That is true when a trusted identity proxy is configured, or when at least one user exists. The check is made on every request and is not cached, so creating the first user tightens access without a restart.

## Identity

- A database user authenticates with a token that is matched by its hash. Every request from a database user is stamped with that user's identity, which is then used both for attribution and as the **viewer** for per-project checks.
- The root token authenticates from configuration. It has no user record, so it is never a viewer, and any grant it makes records no granting user.
- On an install with no database users, no viewer exists and no per-project check applies.

## Projects and access mode

Each project carries an **access mode**: `open` or `restricted`.

- `open` (the default, and the value of every existing project after upgrade) admits any authenticated user. This is the behavior before per-project access.
- `restricted` admits only root, the project's creator, and users holding a grant on it.

A project also records its **creator** when a database user creates it. Projects created by the root token, or on an install with no database users, have no creator. Deleting the creating user clears the creator field; the project is kept.

### Initial mode of a new project

The mode is chosen at creation, by name:

| Project | Initial mode |
|---|---|
| A private user profile (name prefix `_profile.`) | Always `restricted` |
| The default project (`scratch`), the global preferences scope (`_global`, in the default workspace), and a workspace profile (`_profile`) | Always `open` |
| Any other project | The server setting `[auth] new_projects_restricted`. Default `false`, so `open`. |

The setting is applied when the server starts. It never changes existing projects. Existing projects are changed only by an operator, with the project access command.

## Grants

A **grant** gives one user a level on one project. Its fields are: workspace, project, user, level (`read` or `write`), the granting user (empty for a grant made by the root token), and the time it was granted or last changed.

- At most one grant per user per project.
- `read` admits reads in the project: querying, reading pages and observations, status and briefing.
- `write` admits everything `read` admits, plus writing pages, consolidating, handoffs, messages and deletions.
- Project-level admin is not a grant level. Admin stays global and root-only.
- Deleting a workspace, a project or a user removes that user's grants on the deleted entity. Deleting the granting user keeps the grant and clears the granting field.

### Granting and revoking

- **Grant** requires the project to exist; otherwise the request fails with not-found. If the user already holds the same level, nothing is written and the result is "unchanged". If they hold a different level, the level is replaced and the result reports the previous level. Otherwise the result is "granted". Each change is recorded in the audit log as a grant entry with the user, the new level and the previous level.
- **Revoke** removes whatever the user holds. Revoking a user with no grant reports that nothing was revoked and writes no audit entry. A successful revoke is recorded in the audit log. Revoke works with the root token, which has no granting user.
- A grant row that cannot be parsed is skipped and logged. It is treated as no grant, so a malformed row can only deny access, never widen it.

## Decision rule (Implemented)

For a request by a viewer who needs `read` or `write` on a project, the rule is applied in this order:

1. If the deployment does not distinguish operators, allow.
2. If the project is the shared global scope or a workspace profile, and the need is `write`: allow only root or a `write` grant. This applies in either access mode. The creator gets nothing extra. Otherwise refuse with the message "the global scope is shared with every project; writing it requires root or a write grant on it".
3. If the project is `open`, allow.
4. The project is `restricted`. Allow root and the creator. For `read`, allow any grant. For `write`, allow only a `write` grant. Otherwise refuse with the message "project access is restricted; root, the project creator, or a matching grant is required".

Anonymous callers hold no grant and are refused on any restricted project. The refusal names the project, the level needed and the level the caller holds.

A restricted project with zero grants still admits root and its creator, so an operator is never locked out of their own data.

The shared global scope is readable by every user, in every mode. Its pages are unioned into every project's reads, so writing it reaches every user. That is why writes need the stricter rule in step 2. Creating the global scope grants nothing: the first writer does not gain permanent write access.

### Degrade rules

Two resolution paths exist.

- **Standard path** (all scoped reads and writes): if the project row cannot be read, or the project is unknown, the decision degrades to `open` with no creator and a logged warning. If the grant for a restricted project cannot be read, the decision degrades to `open` with a logged warning. For the global scope, an unreadable grant keeps reads open and refuses writes. An access mode value this build does not recognize reads as `open`. This follows the rule that an unresolved reference degrades instead of locking users out.
- **Strict path** (managed-run mutations that release or replace another operator's state: finish, cancel, heartbeat, link, context acceptance): an unknown or unreadable project, an unknown access mode, an unreadable grant, or a user who no longer exists is an error (`authenticated user no longer exists`, a 403 forbidden). The deployment counts as distinguishing operators whenever users exist, even when the caller's level does not say so. The decision itself is the same as the standard path.

The degrade is made per decision, when the data is read. No startup-time check is described here. See [Known gaps](#known-gaps-a-confirmar).

## Where the rule is enforced

### Scoped reads and writes (Implemented)

Every scoped path that carries a viewer resolves its workspace and project through one resolver, which applies the decision rule. Reads are decided on the read path. Writes are decided on the writer's own connection, so a grant change cannot race the write it guards.

Project-resolution operations used by writes (name resolution, alias resolution and identity resolution for a write) also apply the write decision.

### Unscoped reads (Implemented)

Three read paths span every project and do not resolve a single scope:

- the cross-project query (the `global` option of the query tool),
- the global recent-pages listing,
- global web search.

These paths filter the result set **inside the query, before the limit is applied**. The predicate admits a page when its project is:

- open, or
- created by the viewer, or
- one on which the viewer holds any grant, or
- the shared global scope.

Filtering after the limit would leak the existence and count of hits in projects the caller cannot read, and would also leave the visible results short of the limit. A caller with no grants sees exactly the open projects.

When there is no viewer (root, or an install with no database users), no filter is added. The query is then the same as before per-project access existed.

### Raw-id entry points (Implemented, with exceptions)

Some paths take a session, run, handoff or message identifier and find the project from the stored row, without going through a scope. Each of them checks access against the project it resolves to:

| Entry point | Check |
|---|---|
| Consolidation of a named session (dry runs included) | When the caller has a viewer and the session resolves to a target, write access to that target is required before anything runs. |
| Workstream and managed-run reads: status, recent list, event search | Read |
| Workstream and managed-run changes: context, rename | Write |
| Managed-run mutations: finish, cancel, heartbeat, link, context acceptance | Write, using the strict path, plus the owner rule below |
| Handoff delivery and acceptance at session start | Read |
| Sending a message | Write on the recipient project. The recipient must already exist; it is resolved without being created. |
| Listing messages | Read, resolved for the viewer |
| Popping messages | Write, resolved for the viewer, because popping consumes them |

A sent message therefore cannot be used to put text into a restricted project's agents without a write grant on that project. Without that rule, grants could be bypassed through the mailbox.

### Managed-run mutations: owner and legacy policy (Implemented)

A managed run belongs to the operator who opened it. A mutation on a run owned by a different operator is refused as forbidden (`managed run belongs to another operator`), whatever grants that operator holds. **Root is not exempt** (confirmed in the original): the owner test uses the caller's own identity key, and the root level only passes the project check. A root caller whose identity differs from the run's owner is refused with 403, and a root caller with no identity at all (no root username configured) is treated as unattributed and may mutate only runs with no owner. A run with no stored owner (single-operator installs, or opened by an unattributed caller) is admitted for everyone. The owner test runs before the project check, and the refusal never changes the run's state.

The project check for these mutations has two policies:

- **Grants** (the default): the caller must hold write access to the run's project under the strict path.
- **Trusted-proxy legacy**: used only when a trusted identity proxy asserts a user identity that has no database user row. The project check is skipped and only the owner rule applies.

For these checks the deployment counts as distinguishing operators when a trusted proxy is configured, or when the caller is authenticated at the user level.

Any new entry point that accepts a bare identifier must apply the same rule. Otherwise a restricted project's content can be read while scoped tests still pass.

### Ownership of handoffs (Implemented, unchanged)

Handoff ownership filtering stays separate from project grants. Grants decide entry to a project. Handoff ownership decides which handoffs a given operator sees. The two are orthogonal.

## Invariant: grants gate entry, not rows

The per-project check is an allow-or-deny decision for entry to a project. It is never a filter on individual page rows, and page reads are never filtered by the author. A team that holds grants sees the same shared pages as before. The same rule is why the ownership filter of handoffs is not reused for page reads.

## Management surface (Implemented)

All management actions are root-only. A non-root caller receives a forbidden response.

### Commands

| Command | Purpose |
|---|---|
| `user grant` with user, workspace, project and level | Grant a user `read` or `write`, or change their level. The level is required; there is no default. |
| `user revoke` with user, workspace and project | Remove a user's grant on a project |
| `user grants`, optionally with a user | List grants: one user's, or all grants on the server |
| `project grants`, with optional project | List grants on a project |
| `project access` with workspace, project and mode | Set a project to `open` or `restricted`. The mode is required; there is no default. |

The workspace defaults to the default workspace. See [CLI reference](cli.md).

### Admin routes

| Method and path | Purpose |
|---|---|
| POST `/admin/users/{username}/grant` | Grant or change a level |
| POST `/admin/users/{username}/revoke` | Revoke a grant |
| GET `/admin/users/{username}/grants` | One user's grants |
| GET `/admin/projects/grants` | Grants on a project, or on a workspace. Filters are optional. |
| POST `/admin/projects/access` | Set the access mode |

Setting the access mode returns the previous mode, whether it changed, and the list of users who have written to the project but would lose access if it were restricted. That list lets an operator decide whom to grant before restricting. Users who are root, disabled, the creator, or already granted are excluded. The global scope's access mode cannot be changed; the request is refused.

See [API reference](api-reference.md) and [users](users.md).

## Migration and rollout (Implemented)

The change is additive:

- the grants table, the access mode column (default `open`), and the creator column are added;
- every existing project starts `open` with no creator, so nothing changes on upgrade;
- an operator opts a project into `restricted` explicitly, then grants users;
- on a single-operator install, no check applies, so behavior is unchanged.

The identity fields added to projects in a later migration are a separate change. They determine which project a checkout resolves to, which matters because grants attach to projects. They are not part of the access decision itself.

## Limits

- **Creator access is not a grant.** The creator is admitted by the creator rule. Creating a restricted project writes no grant row for its creator. The server setting's description should say so.
- **Degrade is per decision.** Standard-path reads degrade to open when the data cannot be read. Strict-path mutations fail instead.
- **Restricted workspace profiles.** The workspace profile is open by default. An operator can restrict it; reads then follow its access mode, and only writes get the reserved-scope rule.
- **Root and installs without users** bypass the per-project check by design.

## Known gaps (a confirmar)

1. **Requests without a viewer.** The per-project check runs only when a viewer is present. Anonymous requests are rejected on servers where the bearer gate is enabled. On a server where that gate is disabled, anonymous requests reach routes with no viewer. Verified against the reference implementation: the bearer gate is disabled exactly when neither a root token nor human authentication is configured (a stale bearer sent to such a server is treated as anonymous, not rejected); in that state a request carries the anonymous level and no viewer, and the per-project check for id-based routes passes without any lookup. Nothing in the original ties the gate to the existence of database users, so the combination "users exist and the gate is disabled" is not prevented by the bearer layer; Verified against the reference implementation (2026-10-25): user management always refuses an anonymous caller (`user management requires authentication`), even with the gate disabled, so a gate-disabled server cannot create its first user through the API; the combination arises only when users were created while authentication was configured and the root token and human authentication were later removed from the configuration. In that state requests stay anonymous with no viewer (none is added), the per-project check for id-based routes passes without lookup, and the administrative routes refuse the anonymous caller with 401 (`admin operation requires authentication in multi-user mode`), because users exist. Original behavior, reproduce as is.
2. **Page evidence counts.** A bare-identifier operation named "page evidence counts" was described in the original design. No operation with that name exists in the current build. Verified against the reference implementation: there is no bare-identifier entry point that returns evidence counts. The per-page evidence count (rows citing a page version) is only computed inside scoped retrieval (the explained query ranking, the belief/authority score and the "settled first" briefing order), which already runs after the project check. Gap closed: nothing to gate.
3. **Consolidation of a session with no resolvable target.** When the session does not resolve to a target, no write check runs before consolidation. Verified against the reference implementation: the check is skipped when the session has no target and the consolidator then runs; what the consolidator does for a session that resolves to no target belongs to the consolidation audit (feature 020) and is **a confirmar** there. Dry runs are authorized like real runs when a target exists.
4. **Structural enforcement.** Making unguarded lookups private to the module, so that new call sites must go through the guarded resolver, is recommended. Verified against the reference implementation: it is **not** fully applied. The decision itself has a single choke point, and the fail-closed resolver is private to the storage layer, but bare-id lookups without scope or owner predicates remain public operations of the read layer (for example the handoff lookup by id, next to its scoped form, and the session-scope lookup by session id used by hook routing). Their callers are expected to apply the check. Original behavior, reproduce as is (the observable result is the per-call check at each caller).
5. **Global scope grants.** Confirmed in the original: the grant operation itself has no special case for the global scope. It needs only that the project row exists (otherwise `NotFound`), so a `write` grant on the global scope can be created and then admits writes to it. Only changing the global scope's access **mode** is refused.

## Non-goals and proposals

Not implemented, and out of scope for this version:

- per-project admin delegation (project admin remains global and root-only);
- row-level access control within a project;
- time-limited grants;
- groups or roles.

These can be built on the existing grants table later.

## Verification

Behavior is covered by tests in the source for:

- the decision rule across root, granted read, granted write, no grant and anonymous callers, in open and restricted projects;
- the reserved-scope write gate;
- strict and degrade resolution;
- unscoped reads, including that filtering happens before the limit;
- grant, revoke and level change, including audit entries;
- the creator's admission to a restricted project;
- access-mode storage, which degrades to open on an unknown value;
- admin route authorization;
- command parsing for grants and project access.

Open items above should each get a test before the design is considered closed.
