PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE COLLATE NOCASE,
  password_hash TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  csrf_token TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS sessions_expires_idx ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS switches (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  interval_days INTEGER NOT NULL CHECK(interval_days BETWEEN 1 AND 365),
  grace_days INTEGER NOT NULL CHECK(grace_days BETWEEN 0 AND 90),
  state TEXT NOT NULL CHECK(state IN ('ACTIVE', 'OVERDUE', 'RELEASING', 'RELEASED', 'PAUSED')),
  next_deadline_utc TEXT NOT NULL,
  last_checkin_utc TEXT NOT NULL,
  reminder_offsets_json TEXT NOT NULL,
  release_token TEXT NOT NULL UNIQUE,
  recovery_threshold INTEGER NOT NULL DEFAULT 1,
  recovery_share_count INTEGER NOT NULL DEFAULT 1,
  fire_drill_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS switches_state_deadline_idx ON switches(state, next_deadline_utc);

CREATE TABLE IF NOT EXISTS checkin_tokens (
  token_hash TEXT PRIMARY KEY,
  switch_id TEXT NOT NULL REFERENCES switches(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK(kind IN ('magic', 'api')),
  expires_at TEXT,
  used_at TEXT,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS checkin_tokens_switch_idx ON checkin_tokens(switch_id, kind);

CREATE TABLE IF NOT EXISTS keyslots (
  id TEXT PRIMARY KEY,
  switch_id TEXT NOT NULL UNIQUE REFERENCES switches(id) ON DELETE CASCADE,
  type TEXT NOT NULL CHECK(type = 'owner_passphrase'),
  wrapped_dk TEXT NOT NULL,
  iv TEXT NOT NULL,
  salt TEXT NOT NULL,
  kdf_params_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS vault_reservations (
  id TEXT PRIMARY KEY,
  switch_id TEXT NOT NULL REFERENCES switches(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS vault_items (
  id TEXT PRIMARY KEY,
  switch_id TEXT NOT NULL REFERENCES switches(id) ON DELETE CASCADE,
  kind TEXT NOT NULL DEFAULT 'encrypted',
  iv TEXT NOT NULL,
  ciphertext TEXT NOT NULL,
  aad TEXT NOT NULL,
  size_bytes INTEGER NOT NULL CHECK(size_bytes >= 0),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(switch_id, id)
);

CREATE INDEX IF NOT EXISTS vault_items_switch_idx ON vault_items(switch_id);

CREATE TABLE IF NOT EXISTS recipients (
  id TEXT PRIMARY KEY,
  switch_id TEXT NOT NULL REFERENCES switches(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  email TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(switch_id, email)
);

CREATE INDEX IF NOT EXISTS recipients_switch_idx ON recipients(switch_id);

CREATE TABLE IF NOT EXISTS trustees (
  id TEXT PRIMARY KEY,
  switch_id TEXT NOT NULL REFERENCES switches(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  email TEXT NOT NULL,
  share_index INTEGER NOT NULL,
  share_fingerprint TEXT NOT NULL,
  kit_delivered_at TEXT,
  created_at TEXT NOT NULL,
  UNIQUE(switch_id, share_index),
  UNIQUE(switch_id, share_fingerprint)
);

CREATE INDEX IF NOT EXISTS trustees_switch_idx ON trustees(switch_id);

CREATE TABLE IF NOT EXISTS outbox (
  id TEXT PRIMARY KEY,
  switch_id TEXT REFERENCES switches(id) ON DELETE SET NULL,
  kind TEXT NOT NULL,
  to_email TEXT NOT NULL,
  subject TEXT NOT NULL,
  body_html TEXT NOT NULL,
  body_text TEXT NOT NULL,
  scheduled_at TEXT NOT NULL,
  sent_at TEXT,
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS outbox_due_idx ON outbox(sent_at, scheduled_at);

CREATE TABLE IF NOT EXISTS scheduled_events (
  id TEXT PRIMARY KEY,
  switch_id TEXT NOT NULL REFERENCES switches(id) ON DELETE CASCADE,
  event_key TEXT NOT NULL,
  outbox_id TEXT REFERENCES outbox(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL,
  UNIQUE(switch_id, event_key)
);

CREATE TABLE IF NOT EXISTS audit_log (
  id TEXT PRIMARY KEY,
  switch_id TEXT NOT NULL REFERENCES switches(id) ON DELETE CASCADE,
  event TEXT NOT NULL,
  meta_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS audit_log_switch_idx ON audit_log(switch_id, created_at DESC);

CREATE TABLE IF NOT EXISTS fire_drills (
  token TEXT PRIMARY KEY,
  switch_id TEXT NOT NULL REFERENCES switches(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS schema_migrations (
  version TEXT PRIMARY KEY,
  applied_at TEXT NOT NULL
);
