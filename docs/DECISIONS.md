# Architecture decisions

## Zero-knowledge boundary

The browser owns all vault cryptography. The backend accepts opaque ciphertext envelopes and metadata needed to schedule delivery. Recovery shares are transferred owner-to-trustee outside the server through print, local download, copy, or a locally rendered QR code. Share-bearing `mailto:` actions are excluded because mail clients and providers are outside the zero-knowledge boundary.

The account login password is an authentication credential and is hashed server-side with Argon2id. It is intentionally separate from each vault passphrase, which never leaves the browser.

## Reliable release

Scheduler actions first claim a stable event key in SQLite. A unique constraint makes each reminder, recipient release, and trustee instruction idempotent. Email is committed to the durable outbox and delivered by a separate retrying worker. A state change and its audit record share one transaction.

Delivery uses a bounded retry budget. The tenth failure atomically moves a message to a terminal state and appends `email.dead_lettered` without copying the SMTP error or recipient into the audit metadata. The worker emits a structured error containing only the opaque message ID, kind, and attempt count. An authenticated owner can requeue that exact message from the switch Activity tab after fixing SMTP; ownership and terminal state are checked in the same transaction, and `email.requeued` records the action. `/healthz` remains a liveness check so a relay outage does not create a container restart loop.

## Time

Business logic receives a `Clock`. The scaled implementation maps real elapsed time to virtual elapsed time while preserving UTC. Scheduler and sender waits are real durations; all decisions and persisted timestamps come from the injected clock.

## Browser-delivered cryptography

The static application is compiled into the server binary and protected by a same-origin Content Security Policy. This meaningfully reduces supply-chain and third-party-script exposure, but does not prevent a malicious server binary from serving altered JavaScript. `SECURITY.md` documents this unavoidable limitation.
