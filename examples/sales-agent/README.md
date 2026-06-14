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

Phase 0 (this) → Supabase + handoff webhook → live Gmail → SMS + cadences +
scoring → voice + booking + Closer → analytics & hardening. See the project plan
for details.
