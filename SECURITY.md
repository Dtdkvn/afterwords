# Security model

Afterwords is designed around a narrow promise: a correctly built and operated server never receives vault plaintext, the vault passphrase, the Data Key, or trustee recovery shares. This document states what that promise covers and, equally importantly, what it does not.

## Report a vulnerability

Please use GitHub's **Security → Report a vulnerability** flow so maintainers can coordinate a fix before disclosure. Include affected versions, a minimal reproduction, impact, and any suggested mitigation. Do not include real vault content, credentials, or recovery shares.

For an actively exploited deployment, rotate exposed authentication/API credentials, preserve logs without secrets, isolate the instance, and restore only after understanding the compromise.

## Assets and trust boundaries

| Asset | Where it exists | Server visibility |
| --- | --- | --- |
| Account password | Owner browser and transient server memory; Argon2id hash in SQLite | Plaintext during setup/login; hash at rest |
| Vault passphrase | Owner browser memory | Never sent |
| Data Key (DK) | Owner/recipient browser memory | Never sent |
| Trustee shares | Recovery Kits and trustee/recipient browsers | Never sent |
| Vault items | Browser plaintext; ciphertext in SQLite | Ciphertext envelope only |
| Wrapped owner DK | Browser creates it; SQLite stores it | Opaque wrapped value + KDF parameters |
| Contacts and schedules | Browser and SQLite | Visible metadata |
| Session/API/check-in tokens | Browser/client and SQLite | Authentication material |

Afterwords protects content confidentiality from an honest-but-curious server and from disclosure of the database or backups. It does not hide operational metadata such as owner/trustee/recipient email addresses, switch timing, message sizes, audit events, or email delivery records.

## Cryptographic design

- The browser creates a uniformly random 256-bit Data Key with `crypto.getRandomValues`.
- Each vault item uses AES-256-GCM with a fresh random 96-bit IV and AAD exactly equal to `switch_id:item_id`.
- The owner passphrase derives a 256-bit wrapping key with Argon2id and a unique per-switch salt. The server stores the salt, bounded KDF parameters, IV, and wrapped DK.
- For k-of-n recovery with `k >= 2`, the browser uses Privy's zero-dependency `shamir-secret-sharing` implementation, independently audited by Cure53 and Zellic. The 1-of-1 convenience flow is an explicitly labeled encoded recovery share, not threshold splitting.
- The server stores SHA-256 share fingerprints so the browser can reject a mistyped or wrong share before reconstruction. A fingerprint is not a substitute for keeping a share secret.
- Reconstruction is verified by authenticated AES-GCM decryption. Incorrect share combinations produce no plaintext.

The API reserves an item identifier before encryption so AAD cannot drift. Its strict update schema accepts only `iv`, `ciphertext`, `aad`, and `size`; unknown fields such as `plaintext`, `passphrase`, and `share` are rejected. KDF parameters, IV sizes, encodings, AAD, and the per-switch 25 MB ciphertext quota are validated server-side.

During setup, the owner page opens each Recovery Kit with its payload in a URL fragment. Browsers do not send fragments in HTTP requests, and the Kit page removes the fragment from history immediately after bounded parsing. The payload is nevertheless briefly visible to the local browser, extensions, clipboard tooling, and anyone who copies the pre-scrub URL. Use a clean browser profile for setup, print or save the Kit promptly, and close the page afterward.

## The web-delivered cryptography limitation

A malicious or compromised server can serve altered JavaScript that captures a passphrase, Data Key, share, or plaintext. TLS and client-side cryptography do not solve that circular trust problem because the same server delivers the cryptographic client.

v0.1 reduces, but cannot remove, this risk:

- the production frontend is compiled into the same release binary;
- Content Security Policy permits same-origin assets only and no third-party scripts;
- dependency lockfiles make the frontend dependency graph reviewable;
- tag releases are built from the checked-out source with pinned action revisions, locked dependencies, and published checksums; trimmed paths support independent rebuild comparison, but v0.1 does not claim verified bit-for-bit reproducibility;
- the browser performs all cryptographic operations with WebCrypto, bundled Argon2id WASM, and an audited SSS library.

For high-assurance use, build from reviewed source, pin the binary checksum in configuration management, and keep Recovery Kits offline. A signed, single-file offline recipient bundle is a planned mitigation, not a v0.1 claim.

## Server and database compromise

An attacker who obtains the database can learn metadata, crack weak account passwords offline, reuse unexpired sessions/tokens, delete or delay releases, replace ciphertext, and deny service. AES-GCM detects ciphertext modification during decryption, but it cannot guarantee availability or freshness. The attacker does not obtain the vault passphrase, DK, shares, or plaintext from the database alone.

An attacker who controls a running server can also alter the served browser client, email incorrect links, suppress check-ins, or release early. Operational control of the host remains a powerful trust position even when stored content is opaque.

## Authentication and request security

- Owner creation is deliberately available only while the database has no user. Keep a fresh instance on loopback or a private network until setup is complete; the first client reaching an uninitialized public instance could otherwise become the sole owner. `SETUP_TOKEN` adds a constant-time-checked first-boot secret for orchestration, but does not replace network isolation. Generate a high-entropy value, enter it only on the setup form, then remove it from the environment and recreate the service after the owner exists.

- Account passwords are hashed with Argon2id and never logged.
- Sessions use random identifiers in `HttpOnly`, `SameSite=Lax` cookies. Production must set `SECURE_COOKIES=true` and serve only HTTPS.
- Auth endpoints are rate-limited in memory. This slows common online guessing but is not a distributed abuse-control system.
- JSON bodies have explicit size limits and reject unknown fields.
- Browser mutation requests are protected by same-origin checks and session authentication.
- Magic links are short-lived and single-use. API check-in tokens are long-lived and should be rotated after any suspected exposure.
- Afterwords application logs intentionally exclude request bodies, tokens, cookies, email bodies, ciphertext, passphrases, shares, and plaintext. Capability tokens are present in request paths, so reverse-proxy access logs must redact or disable URI logging for `/c/`, `/r/`, `/api/checkin/`, and `/api/release/` routes.

`TRUST_PROXY` must be enabled only when every request passes through a proxy you operate; accepting untrusted forwarding headers weakens client-address-based protections.

## Email and release risks

Email leaks routing metadata and can be delayed, filtered, forwarded, or compromised. Release emails contain a capability URL but never plaintext or a trustee share. Treat the URL as sensitive. Production delivery defaults to mandatory SMTP TLS; a relay that cannot negotiate TLS is rejected instead of silently receiving mail or credentials in plaintext. `opportunistic` explicitly permits fallback to an unencrypted connection and is reserved for a controlled legacy or local relay; `none` is intended only for an unauthenticated local sink such as Mailpit. Both weaker modes require an explicit configuration choice. Use a reputable relay with TLS, SPF, DKIM, and DMARC, and make recipients expect the message before a real release.

The durable outbox provides retry and auditability, not guaranteed delivery. Monitor failures and run fire drills. A trustee instruction email deliberately tells the trustee to use the Recovery Kit obtained outside the server; Afterwords never attaches or relays the share.

## Operational guidance

1. Put Afterwords behind HTTPS and enable secure cookies.
2. Isolate the host, patch it, restrict filesystem access, and do not install unrelated software.
3. Back up the SQLite database and test restoration. Encrypt and access-control backups even though vault content is ciphertext.
4. Keep at least the threshold number of Recovery Kits in separate failure domains.
5. Never photograph or paste real shares into chat, tickets, analytics, or password-manager autofill fields.
6. Leave `TIME_SCALE=1` in production and alert on the startup warning if it changes.
7. Run a complete fire drill after setup, upgrades, SMTP changes, restores, and trustee changes.
8. Rebuild from tagged source and verify published checksums for high-assurance deployments.

## Explicit non-guarantees

Afterwords does not protect against:

- a malicious binary or JavaScript bundle;
- a compromised owner, trustee, or recipient device/browser;
- screen capture, clipboard history, browser extensions, or memory inspection;
- weak vault passphrases or trustees colluding above the threshold;
- loss of too many Recovery Kits;
- host denial of service, clock manipulation, database rollback, or email non-delivery;
- legal enforceability, identity verification, or the correctness of owner-authored content;
- quantum-resistant confidentiality.

The project has not undergone an independent end-to-end security audit. The SSS dependency's audits do not certify Afterwords as a whole.
