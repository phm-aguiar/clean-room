# Arquitetura: partes dependentes de LLM e de embeddings (adiado)

> Trechos removidos de `architecture.md` na separação do escopo inicial (sem LLM,
> local, Docker no Windows). O texto foi preservado como estava. Antes de reativar
> essas funções, revise-o contra o contrato atual.

## Fluxo de auto-melhoria (plano de fundo)

Auto-improvement sits on the provider-backed maintenance side. The server
schedules reviews for newly completed sessions in every project, records
validated proposals in the pending-writes audit trail, and approves them
through the normal wiki write path by default. Scheduler ticks do not overlap:
a long all-project review pass delays the next tick instead of starting
another copy. Scheduling and approval are separate settings. An administrator
can set `[auto_improve.scheduler] enabled = false` to stop background review,
or `[auto_improve] require_approval = true` to leave scheduled and manual
proposals pending for review. Operators can also enable `[auto_improve.eval]`,
which runs a project-supplied executable gate for selected proposal prefixes
after LLM validation and before staging or approval. The gate is disabled by
default and never runs from hook paths.

## Consolidação e auto-melhoria no fluxo de steady-state

4. **Consolidation.** When an LLM provider is configured, `memory_consolidate`
   rewrites the session summary into a richer durable page, or fans it out into
   a multi-page batch under `concepts/`, `decisions/`, and `gotchas/`.
   Consolidation prompts preserve the dominant natural language of the source
   material and ask the model to connect related pages with path-based
   wikilinks. Provider calls for consolidation make up to 3 attempts, 2 s apart.
5. **Auto-improvement.** When an LLM provider is configured, the scheduler
   reviews newly completed sessions across all projects, outside hook latency.
   It records validated proposals for `concepts/`, `decisions/`, `gotchas/`,
   `procedures/`, and `_rules/` in the pending-writes audit trail. It then
   approves them through the wiki mutation path by default. On first run, the
   scheduler records a per-project watermark, so historical sessions are not
   processed automatically on upgrade. It records a per-session claim before
   the LLM work starts, so a failed scheduled review does not retry forever
   (a confirmar: watermark and claim behavior). Explicit CLI, admin, and MCP
   auto-improve calls use the same pipeline for targeted reruns or catch-up.
   With `[auto_improve] require_approval = true`, scheduled and manual
   proposals stay pending until an explicit approval in pending-writes.
   If `[auto_improve.eval] enabled = true`, proposals for the target prefixes
   (by default `_rules/` and `procedures/`) must pass the configured executable
   JSON contract before they are staged. A failed check becomes a rejected
   candidate or a rejection-buffer entry, never a wiki write.
   Every LLM prompt treats repository text, observations, wiki pages, and prior
   proposals as untrusted data, not as instructions. The same trust boundary and
   delimiters precede automatically injected handoffs, project briefs, and
   managed-workstream packets (a confirmar). Current instructions and checkout
   state stay authoritative.

## Reranqueador por LLM (busca)

   An optional reranker (`AI_MEMORY_RERANKER=llm`) sends a bounded query and up
   to 30 bounded titles and snippets to the configured provider, after project
   and scope fusion. It makes at most one call per query, with at most 4 calls in
   flight and a 20 s timeout. Any invalid, failed, timed-out, or saturated
   attempt keeps the local order. Global and supplemental global-preference
   results never take this path.

## Deduplicação por embeddings e passada de sonho

**Near-duplicate clustering (opt-in).** With `[decay] dedup_cold_clusters = true`
(default false), cold episodic pages are clustered by embedding similarity
(density-based clustering with an adaptive epsilon). Each cluster collapses to
one survivor, whose keep-tokens are the union of the cluster's tokens (a
confirmar). The other members are superseded with a merge note. Defaults: minimum points 2 and
maximum epsilon 0.15 (cosine distance). A lower epsilon merges less. Merge
provenance is recorded in the page evidence. The pass uses no generative LLM
and does nothing without an embedder.

**LLM dream pass (opt-in, off by default).** Where clustering collapses
near-duplicates extractively, the dream pass hands each cold cluster to the
configured provider to be rewritten into one coherent page. It runs only when
`[dream] enabled` is set and both a provider and an embedder are configured. A
store without a provider keeps the zero-LLM clustering path. The pass never
deletes a source. The highest-retention member is rewritten, and every
merged-away member is superseded with a merge-note stub, so the full pre-merge
body stays reachable. Provenance for each merge is recorded in the page
evidence. Writes go through the same gated apply path as other wiki writes, with a dry
run available first (a confirmar). Output uses JSON-schema structured output
only. The scheduler starts a run only after `[dream] idle_window_secs` (default
300) with no client activity. Returning activity cancels an in-flight run at the
next cluster boundary (a confirmar). A run processes at most
`max_clusters_per_run` clusters (default 8). Clusters are ordered by novelty, the
most distant from the nearest existing non-cold page first (a confirmar). Each
run returns an observable report, so a bad run is never silent. The pass adds
no migration and no MCP tool (a confirmar: the migration claim).


## Verificação de contradição por embeddings

**Contradiction check (zero-LLM, advisory).** The lint pass also flags cold
semantic and procedural pages whose stored embeddings have a cosine similarity
between 0.4 and 0.75. That band means the same topic but not a near-duplicate
(0.75 and above is the duplicate range, below 0.4 is unrelated). Each such pair
gets a `contradiction` finding with advice that the newer page wins (a
confirmar). The check is bounded to one load of the capped cold set, capped
findings, and deterministic output (a confirmar). With no embedder it is a
no-op. It never deletes, edits, or supersedes a page, and it persists no edge
(a confirmar).

## Ferramentas MCP dependentes de LLM e de embeddings

Linha original de `memory_query` (com reranqueador, resposta e vetores):

| `memory_query` | read-only | Full-text, entity-match, and graph RRF search, plus optional vector RRF, followed by the bounded kind, tier, pinned, and tag authority adjustment and raw fallback. Bumps access counters for page hits. Defaults to the current project. Default-scoped calls also merge the reserved `_global` preferences scope as `global_scope_hits`. `scopes` searches named sibling projects. `global=true` searches every project at once, and each hit is labelled with its workspace and project. With `AI_MEMORY_RERANKER=llm`, project and scopes candidate pools are fused before at most one LLM relevance pass. Timeouts, provider errors, invalid score sets, and saturation keep the adjusted order. The `global=true` ranker and the supplemental global-preference hits are not reranked. `explain=true` adds per-hit score details: per-stream ranks, matched entities, raw scores, RRF contributions, graph provenance including the typed edge kind (`causes`, `fixes`, `contradicts`), the evidence count, the authority multiplier, and the optional rerank score. It also adds a top-level `streams_active` list. `include_expired=true` also returns TTL-expired pages. `include_superseded=true` also returns superseded versions, each labelled `superseded: true`. The current version is never labelled. With the default `false`, the response is identical to the latest-only behavior. `pin_first=true` prepends up to 10 pinned latest pages of the project, deduplicated by page id and marked `pinned: true`, then re-truncates to the requested limit. It applies to single-project searches only. `answer=true` (off by default) adds a cited natural-language answer over the top hits through the configured provider, as `answer: { text, citations }`. With no provider it returns the hits with an `answer_unavailable` note, not an error. With `answer` unset or false, no provider is accessed. The `reasoning` tier (`minimal` default, `low`, `medium`, `high`, `max`) scales the answer's token budget from a base of 2,000: `minimal` 1x, `low` 1.5x, `medium` 2x, `high` 3x, `max` 4x. An unknown tier is rejected by the schema. Answer quality has not yet been evaluated. The response field names in this row are a confirmar. |

| `memory_explore` | read-only | An LLM prose digest over the briefing snapshot. Without a provider it degrades to the structured JSON. The `reasoning` tier scales the digest's token budget from a base of 16,000, with the same multipliers as `memory_query`. |
| `memory_consolidate` | destructive | LLM-driven page rewrite. `multi_page=true` fans out atomically. Without a session id (or with a blank one), it consolidates the latest completed session in the resolved project. If the project has no completed session, it fails with `no completed session in <scope>`. Consolidation prompts append the project's active reserved `_prompts/consolidation.md` page as sanitized, JSON-encoded, untrusted advisory preferences. The page is capped at 2,000 characters. Expired pages are ignored. A per-call `instructions` argument overrides the page for that call. Both system prompts keep the schema, evidence, disclosure, tool-use, and output rules authoritative. |
| `memory_auto_improve` | write | Manually reviews a completed session and applies or stages validated wiki edits through the auto-improvement approval path. Without a session id, it selects the newest completed session with no stored auto-improvement run, so repeated calls advance through preflight skips. An explicit id can always be rerun. |

Linha original de `memory_lint` (com achados por LLM e contradição):

| `memory_lint` | destructive | Rule-based findings plus LLM contradiction findings, written under `_lint/`. It also runs the zero-LLM contradiction check described above, which is advisory only. |

## Invariantes dependentes de LLM e de embeddings

7. **JSON-schema structured output only.** LLM output uses the provider's native
   JSON mode. No XML and no wrapper frameworks.
8. **Embedding provenance.** Each embedding is stored next to its provider, model,
   and dimension. A mismatched vector is warned about and ignored until
   re-embedding completes.
14. **Provider auth resolves before provider construction.** Provider clients
    receive typed authentication material. They never read environment variables
    directly. Token-backed providers receive explicit auth-file paths or
    environment-derived token material, and then manage their own refresh and
    persistence.

## Configuração dependente de LLM e de embeddings

```toml
dedup_cold_clusters = false        # opt-in: collapse near-duplicate cold episodic pages by
                                   # embedding similarity. Zero-LLM, reversible, no-op without an embedder.
# dedup_min_pts = 2                # density floor for clustering (0 = default 2)
# dedup_max_eps = 0.15             # cosine-distance ceiling (0 = default). Lower merges less.
[consolidation]                    # LLM consolidation prompt sizing
max_input_tokens = 100000          # approximate input target. Minimum 6000.
max_output_tokens = 32000          # provider generation limit. Minimum 1000.
                                   # Their sum must fit the model's context window, with headroom
                                   # for tokenizer variance.

[auto_improve]                     # learning reviewer, available by default
require_approval = false           # true leaves proposals pending for review
min_observations = 8
min_session_duration_secs = 120
min_confidence = 0.75
max_input_tokens = 24000
max_proposals_per_run = 5
max_patchable_pages = 8
max_patchable_body_chars = 8000
max_edits_per_proposal = 5
max_edit_content_chars = 4000
max_changed_chars_per_proposal = 12000
max_patch_edits_per_run = 8
max_rejection_context = 50
rejection_context_days = 180
max_final_body_chars = 32000
max_rule_page_tokens = 2000
max_procedure_page_tokens = 2000
include_raw_fallback = false
proposal_actor = "auto_improve"
pending_path = "_pending/auto-improve"

[auto_improve.eval]                # executable gate, disabled by default. Never runs from hook paths.
enabled = false
command = ""                       # project-supplied executable
timeout_secs = 120
targets = ["_rules", "procedures"] # proposal prefixes the gate applies to
min_delta = 0.0

[auto_improve.scheduler]           # background review, separate from approval
enabled = true
interval_secs = 3600
max_sessions_per_tick = 1          # per project. Ticks do not overlap.
min_session_age_secs = 600
experience_every_sessions = 0      # 0 disables the cross-session experience pass
experience_sessions = 10           # session summaries read by one experience pass

[auto_improve.scheduler.experience_entropy_filter]  # opt-in, off by default
enabled = false                    # true skips low-information session pages from the experience
                                   # pass before the prompt, the eval gate, and the apply step.
                                   # Advisory and zero-LLM.
# min_chars = 16                   # near-empty floor (non-whitespace characters)
# min_entropy_bits_per_char = 2.0
# max_repetition_ratio = 0.7       # skip when 1 - distinct/total tokens is above this
# repetition_min_tokens = 6        # the repetition check applies only above this

abstract_vectors = false           # fifth fusion stream over page abstract embeddings (default a confirmar)
[dream]                            # opt-in LLM dream pass. Off by default. Never deletes a source.
enabled = false                    # true starts the scheduled pass, only if a provider and an
                                   # embedder are also configured.
interval_secs = 3600               # how often the scheduler considers a run (0 = 3600; default a confirmar)
idle_window_secs = 300             # quiet period required before a run. Returning activity
                                   # cancels an in-flight run at the next cluster boundary. 0 = 300.
# min_pts = 2                      # density floor (0 = default 2)
# max_eps = 0.15                   # cosine-distance ceiling (0 = default)
# max_clusters_per_run = 8         # clusters per run (0 = default 8)
# min_cold_pages = 2               # skip a run below this many cold pages
```

## Provedores, fallback e variáveis de ambiente

**Fallback chain** (opt-in, TOML only; see
[LLM provider fallback](llm-provider-fallback.md)). The primary provider always runs
first. Fallback entries run only after a transient failure: HTTP 429, a 5xx status,
a timeout, or a connection error. They run in declaration order, and the original
request, schema, and operation id are kept on every attempt. A deterministic
failure (a 4xx other than 429, an unsupported schema, or a malformed response)
stops on the first candidate. The exact set of transient errors beyond the HTTP
statuses is a confirmar. There is no retry loop across the whole chain.

```toml
llm_provider = "opencode"
llm_model = "mimo-v2.5-free"

[[llm_fallbacks]]
provider = "openai-compat"          # same wire names as llm_provider
model = "poolside/laguna-s-2.1-free"
base_url = "http://127.0.0.1:49375/v1"   # required for openai-compat
api_key_env = "AI_MEMORY_LOCAL_ROUTER_TOKEN"  # name of an environment variable. The key itself
                                               # never lives in config.toml.

[[llm_fallbacks]]
provider = "gemini"
model = "gemini-3.5-flash"
api_key_env = "GEMINI_API_KEY"
```

`api_key_env` is required for any provider that needs an API key (anthropic,
openai, gemini, opencode). It is optional for openai-compat, which may run without
a key. It is never inherited from the primary's fixed variable, so a fallback
cannot look configured while resolving no credential. It is optional only for a
provider with a native credential source (openai-oauth, copilot, anthropic-oauth),
which shares the primary's process-wide token material. Configuration validates
every profile and resolves its credential once, at startup. A missing or empty
provider or model, an unknown provider, or a missing credential fails startup.
Each candidate has its own in-memory circuit with a 30 s cooldown. A transient
failure opens it, and a success closes it. A restart clears all circuit state.
There is no durable circuit and no chain-wide deadline. The configuration key
name `llm_fallbacks`, the startup validation, and the status fields described
below are a confirmar.

The server status endpoint (`GET /admin/status`, shown by `ai-memory status`)
reports an `llm_candidates` list alongside the primary `llm` and `embedding`
roles. Each candidate shows its provider and model label, whether it answered the
most recent call, its last success and error timestamps, a redacted error class and
HTTP status, and its circuit-open-until time. The list is empty for a
single-provider setup (a confirmar).

**LLM provider environment** (opt-in):

```text
AI_MEMORY_LLM_PROVIDER     anthropic | anthropic-oauth | openai | openai-oauth | codex | copilot |
                           gemini | openai-compat | opencode
AI_MEMORY_LLM_MODEL        optional when the provider has a default, e.g. claude-haiku-4-5, gpt-5.4-mini
ANTHROPIC_API_KEY / OPENAI_API_KEY / GEMINI_API_KEY / LLM_API_KEY
AI_MEMORY_LLM_BASE_URL     required for openai-compat (Ollama, vLLM). Optional override for
                           opencode (defaults to its Go endpoint; set https://opencode.ai/zen/v1
                           for the Zen catalogue). Applies to any provider, because naming the
                           base URL is how an operator says a vendor endpoint is proxied on purpose.
LLM_BASE_URL               the unprefixed cross-tool convention, accepted for openai-compat and
                           opencode only (a confirmar). Providers with a fixed vendor endpoint ignore
                           it and log why, so a leftover local URL cannot silently redirect other providers.
AI_MEMORY_LLM_COMPAT_STRICT true by default. false disables response_format=json_schema.
AI_MEMORY_LLM_TIMEOUT_SECS per-request timeout for chat providers. 300 by default.
AI_MEMORY_LLM_REASONING_EFFORT optional reasoning effort (none, minimal, low, medium, high,
                           xhigh, max, ultra, persistent). Mapped per provider to the provider's own
                           field. Gemini and Copilot ignore it. Values the host does not support are
                           clamped to the provider's published set (a confirmar: the full value list).
AI_MEMORY_LLM_HEADERS      optional extra HTTP headers on every chat request, as comma-separated
                           Name=Value (or Name: Value) entries. For gateways that need a caller
                           identity header. Headers the server sets itself (authorization,
                           content-type, the provider API key headers, version headers, host,
                           content-length) are refused at startup. Values are never logged.
                           A value cannot contain a comma through the environment variable. Use
                           llm_headers in config.toml for that.
AI_MEMORY_RERANKER         optional, llm. Reranks project and scopes query candidates. Requires a provider.
COPILOT_GITHUB_TOKEN       optional GitHub token for copilot
AI_MEMORY_CODEX_EXECUTABLE optional Codex executable. Defaults to codex on PATH.
GITHUB_COPILOT_API_TOKEN   optional pre-minted Copilot API token
COPILOT_API_URL            optional Copilot API base URL override
```

Every chat request sends `User-Agent: ai-memory/<version>`. Without a configured
agent, HTTP clients send none, and gateways that require caller identification
then treat requests as anonymous. `AI_MEMORY_LLM_HEADERS=user-agent=...` overrides
the default. The Copilot provider keeps its editor-plugin agent string instead, as
the Copilot API expects (a confirmar).

The `openai-oauth` provider uses `auth login openai-oauth` and stores its refresh
token in `<data_dir>/auth.json`. That token is separate from the server's bearer
authentication and from OpenAI platform API keys.

The `codex` provider reads only the access token and account id from the Codex
CLI's own credential file, located through the Codex home setting or the platform
home directory. It never stores Codex credentials itself. A single 401 recovery is
serialized and delegated to the Codex CLI's app server over stdio. Its output is
bounded, and recovery times out after 30 seconds.

The `copilot` provider uses `auth login copilot` or `COPILOT_GITHUB_TOKEN`. It
exchanges the GitHub token for a short-lived Copilot token and calls Copilot Chat
with the editor integration headers. The raw GitHub token is never sent to the
Copilot chat endpoint.

**Embedder environment** (opt-in):

```text
AI_MEMORY_EMBEDDING_PROVIDER   openai | voyage | google | gemini | openai-compat | copilot
AI_MEMORY_EMBEDDING_MODEL      e.g. text-embedding-3-small, gemini-embedding-001
AI_MEMORY_EMBEDDING_BASE_URL   optional override. Required for openai-compat.
AI_MEMORY_EMBEDDING_DIM        1536 (openai, copilot), 1024 (voyage), 768 (google).
                               Required explicitly for openai-compat.
OPENAI_API_KEY / VOYAGE_API_KEY / GEMINI_API_KEY / GOOGLE_API_KEY
LLM_API_KEY                    accepted for openai with a custom base URL, and as optional bearer
                               auth for openai-compat
EMBEDDING_API_KEY              optional embedding-only key. Checked before OPENAI_API_KEY and
                               LLM_API_KEY for the openai and openai-compat embedders.
```

`EMBEDDING_API_KEY` lets the embedding role use a different provider from the
chat model. For example, the chat model can use the public OpenAI endpoint while
vectors come from a cheaper or self-hosted OpenAI-compatible endpoint. Without it,
the openai embedder uses `OPENAI_API_KEY`, then `LLM_API_KEY` when a custom
embedding base URL is set. The voyage and google or gemini embedders read only
their own keys.

openai-compat requires an explicit model and dimension, because self-hosted
engines have no safe default for either. It sends no authorization header when
neither `EMBEDDING_API_KEY` nor `LLM_API_KEY` is set. Its vectors are stored under
the distinct provider identity `openai-compat`.

The copilot embedder takes no API key. It uses the same credential as the copilot
chat provider (`auth login copilot`, `COPILOT_GITHUB_TOKEN`, or
`GITHUB_COPILOT_API_TOKEN`) and shares its token exchange. It defaults to
`text-embedding-3-small` with dimension 1536, and calls the Copilot embeddings
endpoint with the same integration headers as chat. That endpoint's shape follows
the OpenAI-compatible contract Copilot documents for chat. It is not a published
embeddings specification, and no live test against Copilot covers it (a confirmar).

## Trabalho futuro dependente de LLM e de embeddings

* **Local embeddings.** Bundle a small English embedding model for an API-key-free
  path. This adds a large amount to the install size. The provider interface is
  ready. What remains is the local inference backend and tokenizer wiring. See
  [Local embeddings](local-embeddings.md).
* **Vector search extension.** Brute-force cosine similarity works well up to a few
  thousand pages. Beyond that, a native vector search extension is the next step.
  See [Vector backend policy](vector-backend-policy.md) for the criteria that
  would justify adding it.
* **Scheduled consolidation queue.** Forget sweep, lint, and auto-improvement
  already run on server-side schedules. A future queue could compile session
  summaries outside hook latency.

## Leitura complementar dependente de LLM

* [Prompts, schemas and sanitizer](prompts-schemas-and-sanitizer.md): verbatim LLM
  prompts, output schemas, and redaction patterns.
* [Auto-improvement loop](auto-improvement-loop.md): learning-loop design and
  safety boundaries.
