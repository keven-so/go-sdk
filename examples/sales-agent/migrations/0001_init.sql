-- Copyright 2025 The Go MCP SDK Authors. All rights reserved.
-- Use of this source code is governed by an MIT-style
-- license that can be found in the LICENSE file.

-- Phase 1 schema for the sales-agent CRM. Mirrors the entities in
-- internal/crm/models.go so a Supabase/Postgres-backed crm.Store can satisfy
-- the same interface as the in-memory store. Safe to run more than once.
--
-- Apply with the Supabase MCP `apply_migration` tool, the Supabase CLI
-- (`supabase db push`), or psql against DATABASE_URL. See migrations/README.md.

create extension if not exists "pgcrypto"; -- for gen_random_uuid()

-- set_updated_at keeps updated_at current on row updates.
create or replace function set_updated_at() returns trigger as $$
begin
  new.updated_at = now();
  return new;
end;
$$ language plpgsql;

-- leads: a company/opportunity handed off from marketing or generated outbound.
create table if not exists leads (
  id            uuid primary key default gen_random_uuid(),
  source        text not null default 'marketing' check (source in ('marketing','inbound','outbound')),
  company       text not null default '',
  domain        text not null default '',
  status        text not null default 'new' check (status in ('new','working','engaged','meeting','won','lost')),
  score         int  not null default 0 check (score between 0 and 100),
  tier          text not null default '' check (tier in ('','A','B','C')),
  owner_role    text not null default '',
  qualification jsonb not null default '{}'::jsonb,  -- {"bant": {...}, "meddicc": {...}}
  created_at    timestamptz not null default now(),
  updated_at    timestamptz not null default now()
);

drop trigger if exists leads_set_updated_at on leads;
create trigger leads_set_updated_at before update on leads
  for each row execute function set_updated_at();

-- contacts: people at a lead's company.
create table if not exists contacts (
  id            uuid primary key default gen_random_uuid(),
  lead_id       uuid not null references leads(id) on delete cascade,
  name          text not null default '',
  email         text not null default '',
  phone         text not null default '',
  title         text not null default '',
  timezone      text not null default '',
  consent_email boolean not null default false,
  consent_sms   boolean not null default false
);
create index if not exists contacts_lead_id_idx on contacts(lead_id);

-- conversations: one ongoing thread with a contact across channels.
create table if not exists conversations (
  id              uuid primary key default gen_random_uuid(),
  lead_id         uuid not null references leads(id) on delete cascade,
  contact_id      uuid references contacts(id) on delete set null,
  channel_primary text not null default 'email' check (channel_primary in ('email','sms','voice')),
  current_role    text not null default '' check (current_role in ('','sdr','inbound','closer','voice')),
  status          text not null default '',
  gmail_thread_id text not null default ''
);
create index if not exists conversations_lead_id_idx on conversations(lead_id);

-- messages: a single inbound or outbound communication.
create table if not exists messages (
  id              uuid primary key default gen_random_uuid(),
  conversation_id uuid not null references conversations(id) on delete cascade,
  direction       text not null check (direction in ('in','out')),
  channel         text not null check (channel in ('email','sms','voice')),
  external_id     text not null default '',
  subject         text not null default '',
  body            text not null default '',
  created_at      timestamptz not null default now()
);
create index if not exists messages_conversation_id_idx on messages(conversation_id);

-- activities: audit-trail entries (tool calls, stage changes, notes, call results).
create table if not exists activities (
  id              uuid primary key default gen_random_uuid(),
  conversation_id uuid references conversations(id) on delete cascade,
  type            text not null,
  summary         text not null default '',
  payload         jsonb not null default '{}'::jsonb,
  created_at      timestamptz not null default now()
);
create index if not exists activities_conversation_id_idx on activities(conversation_id);

-- deals: sales opportunities tracked through stages.
create table if not exists deals (
  id            uuid primary key default gen_random_uuid(),
  lead_id       uuid not null references leads(id) on delete cascade,
  stage         text not null default 'discovery' check (stage in ('discovery','proposal','negotiation','closed_won','closed_lost')),
  amount        numeric not null default 0,
  won           boolean not null default false,
  qualification jsonb not null default '{}'::jsonb
);
create index if not exists deals_lead_id_idx on deals(lead_id);

-- handoffs: durable idempotency + audit for inbound CustomAIze handoffs. The
-- unique external_id (payload.handoff_id) makes webhook delivery exactly-once:
-- a replayed handoff hits the unique constraint instead of creating duplicates.
create table if not exists handoffs (
  id              uuid primary key default gen_random_uuid(),
  external_id     text not null unique,                -- payload.handoff_id
  source          text not null default '',            -- e.g. customaize
  lead_id         uuid references leads(id) on delete set null,
  conversation_id uuid references conversations(id) on delete set null,
  payload         jsonb not null default '{}'::jsonb,
  created_at      timestamptz not null default now()
);

-- Row-level security: enable on every table. The server connects with the
-- Supabase service-role key (or a direct DATABASE_URL), which bypasses RLS, so
-- no permissive policies are added here. Add per-tenant policies before exposing
-- any of these tables to anon/auth client keys.
alter table leads          enable row level security;
alter table contacts       enable row level security;
alter table conversations  enable row level security;
alter table messages       enable row level security;
alter table activities     enable row level security;
alter table deals          enable row level security;
alter table handoffs       enable row level security;
