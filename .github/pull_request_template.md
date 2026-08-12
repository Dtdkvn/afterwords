## Summary

<!-- What user or operator problem does this change solve? Keep examples fictional. -->

## Changes

-

## Trust boundary and failure behavior

<!--
Explain any effect on browser-only cryptography, plaintext, passphrases, Data Keys,
trustee shares, capability URLs, authentication, metadata, SMTP, outbox retries,
audit events, deadlines, and recovery behavior. Write "No change" where applicable.

Never paste real vault content, credentials, cookies, tokens, release URLs, trustee
shares, personal addresses, ciphertext, or unredacted production logs into a PR.
-->

## Verification

<!-- List exact commands and meaningful manual flows. Do not say "tests pass" without naming them. -->

- [ ] Focused tests cover the changed behavior and its negative/failure paths.
- [ ] `make lint`
- [ ] `make test`
- [ ] `make build`
- [ ] `make e2e` when browser, recovery, state, or delivery behavior changes
- [ ] Production Compose/release checks when packaging or deployment changes

## Documentation and operations

- [ ] User-facing behavior, configuration, and limitations are documented.
- [ ] New logs, audit events, screenshots, fixtures, and test mail contain no secrets or personal data.
- [ ] Database migrations are forward-only, transactional, and exercised from an existing schema when applicable.
- [ ] The change preserves idempotency and leaves terminal failures observable and recoverable when applicable.
- [ ] Security-sensitive details have been moved to a private advisory instead of this PR.
