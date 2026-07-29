# Migrations

Versioned SQL for the sales-agent CRM. These are **not applied automatically** —
they ship as files so the schema is reviewable in the PR and you choose when it
hits a real database.

| File | Description |
|------|-------------|
| `0001_init.sql` | Phase 1 base schema: `leads`, `contacts`, `conversations`, `messages`, `activities`, `deals`, and the `handoffs` idempotency/audit table. Mirrors `internal/crm/models.go`. RLS enabled (server uses the service-role key, which bypasses it). |

Each file is idempotent (`create ... if not exists`), so re-running is safe.

## Apply

Pick one:

**Supabase MCP** (this environment has it connected)
> Use the `apply_migration` tool with name `0001_init` and the file contents.
> Run `list_tables` first to confirm the target project, and `get_advisors`
> after to check for security/perf findings.

**Supabase CLI** (local/linked project)
```bash
supabase db push            # applies migrations to the linked project
# or, for a one-off file:
supabase db execute --file migrations/0001_init.sql
```

**psql / any Postgres**
```bash
psql "$DATABASE_URL" -f migrations/0001_init.sql
```

## Table prefix (sharing a project)

`0001_init.sql` creates the tables in `public` unprefixed, which is ideal for a
**dedicated** database. To drop the schema into a **shared** project's `public`
schema without colliding with existing tables (e.g. a project that already has
its own `leads`), prefix every table name — e.g. `sa_leads`, `sa_contacts` — and
set `SUPABASE_TABLE_PREFIX=sa_` so the Go store targets the prefixed names. The
prefix keeps everything in `public`, which is the schema PostgREST exposes by
default, so no API-settings change is needed.

## Recommended order

1. Apply `0001_init.sql` to a **non-production** Supabase project (or a shared
   project with a table prefix, see above).
2. Set `SUPABASE_URL` + `SUPABASE_SERVICE_ROLE_KEY` (and `SUPABASE_TABLE_PREFIX`
   if you prefixed) — `salesctl serve` then uses the Supabase-backed `crm.Store`
   (`internal/crm/supabase.go`) automatically instead of the in-memory store.
3. POST a handoff and confirm rows land in the database.

Without those env vars the webhook intake runs against the in-memory store, so
the contract and end-to-end flow stay testable with no database.
