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

## Recommended order

1. Apply `0001_init.sql` to a **non-production** Supabase project first.
2. Wire a Supabase-backed `crm.Store` (Phase 1 follow-up) using
   `SUPABASE_URL` + `SUPABASE_SERVICE_ROLE_KEY` (or `DATABASE_URL`).
3. Point `salesctl serve` at it instead of the in-memory store.

Until then the webhook intake runs against the in-memory store, so the contract
and end-to-end flow are testable without a database.
