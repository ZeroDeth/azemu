# Security Policy

## Supported versions

| Version | Supported |
|---------|-----------|
| latest on `main` | Yes |
| tagged releases (v0.x) | Yes |
| older than latest tag | No |

## Reporting a vulnerability

**Do not open a public issue.** Instead, use one of these private channels:

1. **GitHub Security Advisories** (preferred): go to
   [Security > Advisories](https://github.com/ZeroDeth/azemu/security/advisories)
   and click "New draft security advisory".
2. **Email**: send details to **<REDACTED>**.

Include:

- azemu version (or commit hash).
- Steps to reproduce.
- Impact assessment if you have one.

## Response time

- Acknowledgement within 48 hours.
- Triage and severity assessment within 7 days.
- Fix or mitigation published as a patch release once ready.

## Scope

azemu is a local emulator. It does not connect to Azure, hold real
credentials, or manage production data. Vulnerabilities that matter most
are those that could affect the host machine running azemu (for example,
path traversal via crafted ARM resource IDs, or TLS certificate
mishandling).

The self-signed TLS certificate is generated at runtime and stored in
`.azemu/cert-bundle.pem` (gitignored). It is not a secret, but it should
not be committed to version control.
