# Afterwords progress

Last updated: 2026-08-12

## Public release

- Public repository: <https://github.com/Dtdkvn/afterwords>
- Current release: [`v0.1.0`](https://github.com/Dtdkvn/afterwords/releases/tag/v0.1.0), published 2026-08-12
- Release assets: macOS arm64 and Linux amd64/arm64 archives, SHA-256 checksums, and one SPDX 2.3 SBOM per archive
- Hosted release verification: Go race tests and lint, frontend tests/build, the real Mailpit recovery flow, archive-contract checks, and GoReleaser all passed on the tagged commit

## Milestones

| Milestone | Status | Notes |
| --- | --- | --- |
| M0 — Scaffold | Complete | Go/React repository, embedded production frontend, CI, developer Compose, Mailpit, and release tooling. |
| M1 — Engine | Complete | SQLite store, injected/scaled clock, fail-safe state machine, three check-in methods, durable outbox, SMTP retry, cleanup, and transactional audit log. |
| M2 — Crypto | Complete | Browser-only AES-GCM, Argon2id wrapping, audited Shamir split/combine, strict ciphertext/keyslot APIs, and crypto tests. |
| M3 — Release | Complete | Guided recipient recovery, offline Recovery Kits, share paste/type/file import, fire drill, and Playwright vertical flow. |
| M4 — Ship-ready | Complete | Polished responsive UI, containers, GoReleaser, CI, security and contributor docs, demo scripts, and local verification. |

The owner explicitly authorized uninterrupted autonomous execution across milestone boundaries for this build. Milestone approval pauses from the original workflow were therefore waived.

## Decisions

- Kept the working name **Afterwords**. A 2026-08-11 check found unrelated GitHub repositories with that word, but no exact npm package and no official Docker Hub image; nothing found occupied this product niche.
- The canonical Go module is `github.com/Dtdkvn/afterwords`, and the canonical public repository is <https://github.com/Dtdkvn/afterwords>.
- The server stores session credentials and the account login password hash, but never the vault passphrase, Data Key, trustee shares, or plaintext vault data.
- Recovery Kit shares are created in the browser and can be printed, downloaded, copied, or encoded into a locally generated QR code. Share-bearing email actions were deliberately removed because a mail client/provider is outside the zero-knowledge boundary. The server emails instructions only and never a share.
- Trustee share fingerprints are SHA-256 hashes of the decoded share bytes. They validate a submitted share without revealing it.
- The audited `shamir-secret-sharing` package by Privy is used for thresholds of at least 2. The specified 1-of-1 convenience path stores the Data Key as a versioned recovery share; this is explicitly not presented as threshold splitting.
- Vault item identifiers are reserved before encryption so the required AAD can be exactly `switch_id:item_id`. The subsequent strict PUT body accepts only `iv`, `ciphertext`, `aad`, and `size`.
- SQLite is the only supported database in v0.1. Scheduler idempotency is enforced with a unique `scheduled_events` key and conditional state transitions.
- A fire drill receives its own short-lived public token and outbox message, but does not alter the switch state, deadline, or real release token.
- Magic-link GET requests are side-effect-free confirmation pages so email scanners cannot reset a deadline; the one-use token and check-in are committed together only on explicit POST.
- Release fails closed until a switch has an owner keyslot, ciphertext, a recipient, a consistent k-of-n trustee set, and every Recovery Kit marked opened. Release payload and people mutations are frozen once release begins.
- Optional `SETUP_TOKEN` protects first-owner creation with a constant-time comparison; network isolation during first boot remains the primary control.
- SMTP requires TLS by default. Opportunistic downgrade and plaintext modes must be explicitly selected, and plaintext mode rejects SMTP credentials.

## Dependency rationale

- `chi`: small, idiomatic HTTP routing and middleware.
- `modernc.org/sqlite`: pure-Go SQLite keeps release binaries CGO-free.
- `wneessen/go-mail`: robust multipart SMTP delivery as required by the product spec.
- `golang.org/x/crypto/argon2`: standards-based Argon2id account-password hashing.
- `hash-wasm`: browser Argon2id implementation; its WebAssembly is bundled locally.
- `shamir-secret-sharing`: zero-dependency implementation independently audited by Cure53 and Zellic.
- `react-markdown`: renders owner-authored Markdown without enabling raw HTML.
- `qrcode`: generates local Recovery Kit QR codes without a network service.

## Verification

- `go test -race -cover ./...` (the Makefile gate), `go vet ./...`, `go mod verify`, and a trimmed production binary build pass. Verified with Go 1.25.12 and 1.26.5; the module minimum is Go 1.25. The race detector reports no data races across 55 tests.
- `govulncheck ./...` reports zero reachable Go vulnerabilities after upgrading `go-mail`, `x/crypto`, and `x/text`; Docker Scout reports zero critical/high findings in the final image.
- Frontend TypeScript checks, 9 Vitest crypto/fragment/secure-context tests, and the Vite 8 production build pass.
- Accessibility: axe-core (`wcag2a`, `wcag2aa`, `wcag21a`, `wcag21aa`) reports zero violations on the authentication, recipient, dashboard, and wizard screens at 1280x800 and 375x812 in both themes, including the recipient page after a share is entered. Contrast was measured from the palette tokens rather than inferred: secondary text clears 5.27:1 and accent fills clear 5.26:1 at their worst pairing.
- Playwright 1.62 completes owner setup → encrypted vault → Recovery Kits → scaled deadline release through SMTP/Mailpit → recipient share reconstruction → plaintext display.
- Full and production-only npm audits report zero vulnerabilities for both `web` and `e2e` lockfiles.
- Production and development Compose files pass `docker compose config --quiet` validation. The production image builds locally, runs as the unprivileged `afterwords` user with a read-only root filesystem/capabilities dropped, and serves both `/healthz` and the embedded SPA.

## Known limitations

- The local container smoke covered Linux/amd64. Other architectures are produced and checked by the release/CI configuration but were not executed on physical arm64 hardware in this audit.
- The real scaled deadline → SMTP/Mailpit → recipient recovery flow passed locally with portable Mailpit. Deterministic fake-clock tests additionally cover the state-machine boundaries.
- The complete application has not received an independent cryptographic audit. The Shamir dependency has independent Cure53 and Zellic audits; the web-delivery trust limitation is documented in `SECURITY.md`.

## Open questions

No questions block v0.1. Product follow-ups intentionally remain outside scope: multiple owners, WebAuthn, messaging integrations, offline signed recipient bundles, and external object storage.
