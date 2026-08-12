# Contributing to Afterwords

Thank you for helping make a sensitive workflow calmer and safer. Correctness, honesty, and reviewability take priority over feature count.

## Ground rules

- Use English for code, comments, UI copy, documentation, issues, and commits.
- Keep changes focused and use Conventional Commits (`feat:`, `fix:`, `test:`, `docs:`, `chore:`).
- Do not put real credentials, vault content, passphrases, API tokens, recovery shares, or personal email addresses in fixtures or screenshots.
- Discuss security vulnerabilities through a private GitHub Security Advisory, not a public issue.
- v0.1 is intentionally single-owner, SQLite, email-only, and local-storage-only. Open an issue before expanding those boundaries.

## Non-negotiable invariants

1. The server never receives, stores, or logs vault plaintext, a vault passphrase, the Data Key, or a recovery share.
2. Vault endpoints accept strict ciphertext-only schemas and reject unknown fields.
3. Business time comes from an injected `Clock`; do not call `time.Now()` in business logic.
4. Handlers never send email inline. They commit a durable outbox row for the sender worker.
5. Every state transition is idempotent and writes an audit row in the same transaction.
6. No third-party script origin may be added to Content Security Policy.
7. Recovery Kit shares leave the browser outside the Afterwords server.

A contribution that weakens an invariant must not be merged, even if it makes a demo easier.

## Set up

Prerequisites are Go 1.25+, Node.js 24+, npm, GNU Make, Bash, standard POSIX utilities, `curl`, and Docker for Mailpit/demo tests. Windows contributors should use WSL or an equivalent environment with these tools.

```bash
git clone https://github.com/yewud/afterwords.git
cd afterwords
make setup
make test
```

The URL above is the expected canonical publication path. This local build has no configured remote until the repository is explicitly created and pushed.

Run the development stack with `make dev`. The UI is at `http://localhost:5173`, the API at `http://localhost:8080`, and Mailpit at `http://localhost:8025`.

## Before opening a pull request

```bash
make lint
make test
make build
make e2e
```

Also verify the scenario affected by your change with `TIME_SCALE=86400 make demo`. UI changes should be checked on mobile and desktop, in light and dark mode, with keyboard navigation and visible focus.

Pull requests should explain:

- the user-visible problem and chosen behavior;
- any trust-boundary or data-model change;
- tests added, including failure and retry paths;
- dependency rationale and security/audit status;
- screenshots for UI work, using fictional data only.

## Backend style

- Prefer the standard library. Approved core dependencies are chi, modernc SQLite, go-mail, and `golang.org/x/crypto` for Argon2id; explain every additional dependency.
- Use table-driven tests and temporary SQLite databases.
- Thread `context.Context` through I/O and keep transactions short.
- Use stable event/idempotency keys for scheduler and outbox work.
- Validate at the HTTP boundary and keep public error messages free of internal or secret data.
- Run `gofmt`, `go vet`, golangci-lint, and the race detector.

## Frontend and cryptography style

- Keep cryptographic primitives in the small `web/src/lib/crypto.ts` boundary and cover every path with Vitest.
- Use WebCrypto AES-GCM, bundled Argon2id WASM, and the audited SSS dependency; do not implement cryptographic primitives locally.
- Generate a fresh 96-bit IV for every item and bind `switch_id:item_id` as AAD.
- Clear sensitive component state when leaving a flow where practical. Never persist plaintext or shares in localStorage, sessionStorage, query parameters, server-visible URLs, analytics, or logs. The Recovery Kit's fragment is a narrowly scoped local handoff: parse it once, enforce a small size/schema, and immediately scrub it from browser history with `history.replaceState`.
- Do not enable raw HTML in Markdown rendering.
- Prefer small accessible components over a UI framework. Recipient and Recovery Kit flows must work on narrow screens and print cleanly.

## Tests expected for sensitive changes

- State machine: fake-clock transition, retry, duplicate-tick, pause/resume, and terminal-state cases.
- Vault API: strict unknown-field rejection, malformed encodings, wrong AAD, quota, and ownership.
- Crypto: encrypt/decrypt round trip, wrong passphrase, k-of-n success, k-1 failure, duplicate/bad share, and 1-of-1 labeling.
- Email: outbox durability, retry/backoff, idempotent scheduling, fire-drill recipient restriction.
- Release: only released/test tokens resolve, ciphertext bundle contains no private metadata, decryption stays in-browser.

## Database migrations

Migrations are embedded, sequential, immutable after release, and applied transactionally. Add a new numbered migration instead of editing one that has shipped. Include upgrade and fresh-database tests.

## Release snapshots

Use GoReleaser v2.17.1 with Go, Node.js, and npm available on `PATH`:

```bash
goreleaser check
goreleaser release --snapshot --clean
bash scripts/check-release-archives.sh dist
```

The GoReleaser configuration installs the locked frontend dependencies, rebuilds the embedded application, and refuses to package it if `internal/server/webdist` differs from the committed output. This makes a manual release enforce the same fresh-frontend invariant as CI.

## Dependencies

Avoid adding dependencies for short functions. For a necessary dependency, document why it is needed, its maintenance state, license, browser/server footprint, and any relevant security audit. Commit lockfile changes and inspect install scripts before merging.

## Documentation

Update README, configuration tables, security claims, API examples, and `PROGRESS.md` with the code. Never claim an audit, delivery guarantee, reproducible build, or zero-knowledge property that the implementation does not demonstrate.
