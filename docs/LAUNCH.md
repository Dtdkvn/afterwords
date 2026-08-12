# Afterwords launch kit

Use this page as copy-ready material for the public GitHub launch. Verify the checklist against the exact commit being published, and keep the limitations as visible as the feature claims.

## GitHub metadata

### About description

> Self-hosted dead man's switch with browser-only vault encryption, trustee recovery, a durable email outbox, and a calm recipient flow.

### Suggested topics

`dead-mans-switch`, `self-hosted`, `client-side-encryption`, `digital-legacy`, `privacy`, `go`, `react`, `sqlite`, `docker`, `shamir-secret-sharing`, `webcrypto`, `email`

### One-line pitch

> Afterwords releases client-encrypted messages after missed check-ins, while trustee shares and decryption stay outside the server.

### Hacker News title

> Show HN: Afterwords, a self-hosted client-encrypted dead man's switch

## Long launch post

Today I am sharing Afterwords, an open-source, self-hosted dead man's switch for messages and files that should reach chosen people only after you stop checking in.

The basic idea is familiar: set a schedule, check in periodically, and notify someone if the deadline passes. The difficult part is making that workflow useful without turning the server into a vault that an operator, attacker, or leaked backup can simply read. It also has to work for recipients and trustees who should not need a terminal, a crypto background, or an account.

Afterwords starts with a single owner creating a switch, choosing a check-in interval and grace period, and adding recipients and trustees. The owner's browser generates a random Data Key and encrypts each vault item with AES-256-GCM. A passphrase-derived Argon2id key wraps that Data Key for owner access. The browser then creates a k-of-n trustee recovery set. The server receives ciphertext, the wrapped key, and share fingerprints, but not the vault passphrase, raw Data Key, trustee shares, or plaintext.

Each trustee gets a printable or downloadable Recovery Kit outside the server. If the switch is eventually released, recipients receive a capability link and ask enough trustees for their shares. Share validation, key reconstruction, authenticated decryption, Markdown rendering, and file downloads all happen in the recipient's browser. The UI presents this as a calm numbered flow instead of a collection of cryptographic primitives.

The release path is deliberately inspectable. Switches move through ACTIVE, OVERDUE, RELEASING, and RELEASED states. State changes are idempotent and written to an audit history. Emails are committed to a durable SQLite outbox before sending. Failed deliveries are retried up to ten times. A terminal failure remains visible in the switch Activity view, where the owner can correct the SMTP problem and explicitly retry it. A release is still only as reliable as the host, database, network, and mail provider, so Afterwords includes fire drills and makes delivery failures visible rather than claiming guaranteed email.

There are several details aimed at preventing ordinary automation from doing surprising things. Previewing a magic check-in link is side-effect-free because mail scanners often open links. The owner must confirm the check-in with a POST. API check-in tokens can be rotated. A fire drill sends test content only to the owner and does not change the real switch state or deadline. For local demonstrations, accelerated time compresses days into seconds, while production warns whenever the time scale is not one.

The security boundary is intentionally narrow and documented. Afterwords protects vault content from disclosure of the SQLite database or backups, and from an honest-but-curious server that runs the reviewed build. It does not solve the central limitation of web-delivered cryptography: a compromised server can serve modified JavaScript that steals secrets from the browser. The server also sees operational metadata such as email addresses, schedules, sizes, and audit events. The project has not undergone an independent end-to-end security audit. Audits of one cryptographic dependency do not certify the application as a whole.

Operational care still matters. Remote access requires HTTPS because browser cryptography needs a secure context. The production SMTP policy requires TLS by default, but mail can still be filtered or delayed. A new public instance must be protected during first-owner setup. Backups remain sensitive even though vault content is encrypted. Recovery Kits need to be kept offline and in separate failure domains. Afterwords is safety-adjacent software, not a replacement for a will, legal advice, emergency services, or a tested estate plan.

The first release is intentionally focused: one owner, one SQLite process, email delivery, local storage, and browser-based recovery. It does not claim high availability, multiple accounts, mobile apps, S3 storage, generic webhooks, or a signed offline recipient bundle. Release automation is configured for CGO-free Linux amd64/arm64 and macOS arm64 archives, with SHA-256 checksums and an SBOM for each archive.

You can try it locally with:

```bash
git clone https://github.com/Dtdkvn/afterwords.git
cd afterwords
docker compose up --build
```

The default stack opens on `http://localhost:8080` and intentionally has no real SMTP relay configured. Use the bundled Mailpit demo to exercise the complete flow locally, then configure HTTPS, SMTP, backups, and monitoring before treating an instance as operational.

I would especially value review of the [security model](../SECURITY.md), the recovery instructions for non-technical people, and failure cases around email and long-running self-hosted deployments. The [README](../README.md), [architecture decisions](DECISIONS.md), [contribution guide](../CONTRIBUTING.md), and [demo recording guide](../scripts/record-demo.md) provide the implementation and testing details.

## Short post

I built Afterwords, an open-source, self-hosted dead man's switch for encrypted messages and files. You check in on a schedule; after a missed deadline and grace period, recipients get a release link and trustees provide enough Recovery Kit shares to decrypt in the recipient's browser.

The server stores ciphertext and operational metadata, but a correctly built and operated instance never receives vault plaintext, the vault passphrase, the Data Key, or trustee shares. Delivery uses a state machine with audit history and a durable SQLite outbox, with visible terminal failures and owner-initiated retry. Fire drills and accelerated demo time make the full lifecycle testable.

The security model is deliberately honest: a compromised server can still serve malicious JavaScript, email is not guaranteed, and the project has not had an independent end-to-end security audit. v0.1 is single-owner, SQLite, email-only, and MIT licensed.

Repository: https://github.com/Dtdkvn/afterwords

## Launch checklist

### Repository and profile

- [ ] Push the reviewed `main` branch to <https://github.com/Dtdkvn/afterwords> and verify it is the default branch.
- [ ] Confirm `github.com/Dtdkvn/afterwords` is the module path used by `go.mod`, Go imports, documentation, and release metadata.
- [ ] Set the GitHub About description and suggested topics from this page.
- [ ] Confirm GitHub recognizes the MIT license and displays the CI workflow.
- [ ] Enable private vulnerability reporting before linking the Security Advisory flow.
- [ ] Configure branch protection and require the full CI suite for changes to `main`.
- [ ] Add a social preview image made only with fictional names, messages, addresses, tokens, and shares.

### Evidence before announcement

- [ ] Run `make lint`, `make test`, `make build`, and `make e2e` from a clean checkout with the documented toolchain.
- [ ] Run `docker compose config --quiet`, build the production stack, and smoke-test `/healthz` plus the embedded web app.
- [ ] Run a GoReleaser snapshot and inspect every archive for the binary, documentation, checksums, and per-archive SBOM.
- [ ] Test the quick start from a fresh clone with no warm Go, npm, or Docker cache assumptions.
- [ ] Configure a real SMTP relay in a disposable environment and verify reminder, trustee, release, failure, dead-letter, and manual retry behavior.
- [ ] Complete a full Recovery Kit fire drill with fictional data and confirm the real switch state and deadline do not change.
- [ ] Review [SECURITY.md](../SECURITY.md) and [README.md](../README.md) for claims that exceed the tested commit.
- [ ] Scan the repository, built frontend, container history, logs, screenshots, and demo mail for secrets or personal data.

### Release and launch assets

- [ ] Record a short product walkthrough using [the demo shot list](../scripts/record-demo.md).
- [ ] Create a release tag only after CI passes, then let the release workflow create a draft.
- [ ] Verify the draft archives, checksums, SBOMs, supported platforms, and embedded frontend before publishing the release.
- [ ] Capture the final README and demo at desktop and mobile widths.
- [ ] Post the long version where context is welcome, use the Hacker News title above, and use the short version for compact channels.

### After launch

- [ ] Watch CI, installation reports, and SMTP/recovery feedback during the first 48 hours.
- [ ] Move suspected vulnerabilities to private reporting immediately and keep real secrets out of public issues.
- [ ] Reproduce bug reports with fictional data, document confirmed limitations, and avoid promising timelines before the scope is understood.
- [ ] Keep the security model and operational checklist synchronized with every behavior change.

## First release workflow

The first release tag is `v0.1.0`. Tag only the exact commit that is already present on `origin/main`; the release workflow rejects tags that are not ancestors of the remote default branch.

From a clean, verified `main` checkout after its CI run succeeds:

```bash
git tag --annotate v0.1.0 --message "Afterwords v0.1.0"
git push origin refs/tags/v0.1.0
```

Pushing the tag starts `.github/workflows/release.yml`. The workflow validates the tag and `main` ancestry, installs and audits locked dependencies without release credentials, runs Go and frontend tests, builds the embedded frontend, runs the Playwright release flow against Mailpit, and builds and inspects a GoReleaser snapshot. Only the final GoReleaser invocation receives `GITHUB_TOKEN`, and it skips dependency hooks. GoReleaser then creates a **draft** GitHub Release containing Linux amd64/arm64 and macOS arm64 archives, SHA-256 checksums, and one SBOM per archive.

Review the draft assets and release notes before publishing it. v0.1.0 artifacts are checksummed but not signed, and the project does not claim verified bit-for-bit reproducible builds.
