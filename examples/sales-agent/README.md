# Sales Agent (Phase 0 skeleton)

A multi-agent AI sales system built on the MCP Go SDK. AI sales agents take over
after marketing sends the first email and work each lead to close across email,
SMS, and phone. The "brain" is a **Claude agent loop in Go**; sales capabilities
are exposed as **MCP tools**; data lives in **Supabase** (later phases).

This directory is its own Go module (it depends on the Anthropic SDK, which the
core `go-sdk` module does not). It uses a `replace` directive to build against
the SDK in this repo.

## What Phase 0 delivers

- **Agent loop** (`internal/agent/loop.go`) — drives an LLM, executes the tools
  it requests, feeds results back until done.
- **Tool bridge** (`internal/agent/toolbridge.go`) — the seam that maps the
  model's `tool_use` calls onto MCP `CallTool` over `ClientSession`. This is the
  core idea: the loop is an **MCP client**; capabilities are **MCP servers**.
- **Six MCP tool servers**: four base servers with dry-run providers (`comms`
  (email/SMS/voice), `crm`, `schedule` (booking), `intel` (enrich/score)) and two
  higher-order servers (`team` for support sub-agents, `orchestrator` for routing
  and qualification).
- **In-memory CRM store** (`internal/crm`) implementing the `Store` interface a
  Supabase backend will later satisfy.
- **Provider-agnostic LLM** (`internal/llm`): a real Anthropic adapter **and** a
  scripted fake, so the whole pipeline runs with no API key and no network.
- **`salesctl dry-run`** — seeds a marketing handoff and runs the SDR agent
  end-to-end, sending nothing real.

## Run it

```bash
cd examples/sales-agent
go mod tidy
go test ./...
go run ./cmd/salesctl dry-run            # scripted, no API key, no sends
go run ./cmd/salesctl dry-run -live      # real Claude (needs ANTHROPIC_API_KEY)
```

`dry-run` prints each agent step (tool calls + results) and the resulting CRM
activity log. With `-live`, the same loop runs against the real Claude API while
the comms providers stay in dry-run, so still nothing is actually sent.

## Phase 1: CustomAIze handoff + Supabase schema

CustomAIze runs marketing and sends the first email; when it hands a lead to
sales it POSTs a **handoff** to this service. Phase 1 adds the intake and the
database schema that persists it.

- **Handoff webhook** (`internal/intake`) — `POST /webhooks/handoff` verifies an
  HMAC-SHA256 body signature (`X-CustomAIze-Signature: sha256=<hex>`, keyed by
  `WEBHOOK_SIGNING_SECRET`), then creates the lead, contact, conversation, the
  already-sent first email, and a `handoff_received` activity, and routes the lead
  to a customer-facing role via the supervisor. Delivery is **idempotent** on
  `handoff_id` (replays return the original ids, write nothing).
- **Schema** (`migrations/0001_init.sql`) — `leads`, `contacts`, `conversations`,
  `messages`, `activities`, `deals`, plus a `handoffs` table whose unique
  `external_id` gives durable exactly-once delivery. Files only; see
  [`migrations/README.md`](migrations/README.md) for how/when to apply.

```bash
go run ./cmd/salesctl serve        # listens on $HTTP_ADDR (default :8080)

# example handoff (computes the signature from WEBHOOK_SIGNING_SECRET):
body='{"handoff_id":"ho_1","source":"customaize",
  "lead":{"company":"Acme","domain":"acme.com"},
  "contact":{"email":"jane@acme.com","name":"Jane","consent_email":true},
  "first_email":{"subject":"Hi","body":"...","gmail_thread_id":"t1"}}'
sig="sha256=$(printf '%s' "$body" | openssl dgst -sha256 -hmac "$WEBHOOK_SIGNING_SECRET" | awk '{print $2}')"
curl -sX POST localhost:8080/webhooks/handoff \
  -H "X-CustomAIze-Signature: $sig" -d "$body"
```

- **Supabase-backed store** (`internal/crm/supabase.go`) — a `crm.Store`
  implementation over Supabase's PostgREST API using the service-role key. When
  `SUPABASE_URL` + `SUPABASE_SERVICE_ROLE_KEY` are set, `salesctl serve` uses it
  automatically; otherwise it falls back to the in-memory store, so the contract
  and end-to-end flow stay testable with no database. `SUPABASE_TABLE_PREFIX`
  (e.g. `sa_`) namespaces the tables when sharing a project's `public` schema.

The exact CustomAIze payload may differ — `HandoffPayload` in
`internal/intake/intake.go` is the contract to adjust, and `Context` carries
arbitrary marketing metadata so the schema need not change to add fields.

## Architecture (hierarchical supervisor + agent-as-tool)

The roster is grounded in current production AI sales systems (Alta Katie/Alex/Luna,
Salesforce Agentforce SDR + Sales Coach, 11x/Landbase hierarchical multi-agent):

- **Supervisor** routes each event to a **customer-facing** agent: **Outbound SDR**,
  **Inbound Qualifier**, **Voice/Calling**, **AE/Closer**.
- **Support agents** — **Researcher**, **Copywriter**, **RevOps/Intelligence**,
  **Sales Coach/QA** — are exposed to the others as sub-agent **tools** (the
  agent-as-tool pattern via the `team` server).
- Role transitions use an explicit `handoff(to_role, reason)` tool; qualification
  uses **BANT** (SDR/Inbound) and **MEDDICC** (Closer), persisted via
  `update_qualification`. See `internal/agent/roles.go`; run `salesctl roles`.

Each customer-facing "team member" is the **same `agent.Loop`** parameterized by a
`Role` (system prompt + allowed tools). Two tool tiers break the agent-as-tool
cycle: an **inner** bridge (crm + intel) for support sub-agents, and an **outer**
bridge (all servers) for customer-facing agents.

```
salesctl ─▶ Supervisor.Route ─▶ agent.Loop ─▶ ToolBridge ─▶ mcp.ClientSession.CallTool
   (brain: Anthropic │ Fake)                        │
        ┌───────────┬───────────┬───────────┬───────┴────┬──────────────┐
      comms        crm       schedule      intel        team         orchestrator
   (email/sms/  (Supabase   (calendar)   (enrich/   (research/draft/  (route/handoff/
    voice,dry)   later)                   score)     prioritize/coach)  qualification)
                                                     = support sub-agents
```

## Roadmap

Phase 0 (skeleton) → **Phase 1: Supabase schema + handoff webhook (this)** →
live Gmail → SMS + cadences + scoring → voice + booking + Closer → analytics &
hardening. See the project plan for details.

Phase 1 remaining: apply `migrations/0001_init.sql` to a Supabase project and set
the `SUPABASE_*` env vars; `salesctl serve` then persists handoffs there via the
Supabase-backed `crm.Store`.
