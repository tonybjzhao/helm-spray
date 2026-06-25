# Security Policy

## Supported versions

Security fixes are provided for the latest `4.x` release of helm-spray.

| Version | Supported |
|---------|-----------|
| 4.x     | ✅        |
| < 4.0   | ❌        |

## Reporting a vulnerability

Please **do not** open a public issue for security vulnerabilities.

Report them privately through GitHub's
[private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)
on this repository (Security → Report a vulnerability), or by contacting the
maintainers directly.

Please include:

- a description of the issue and its impact,
- the affected version(s) and environment (helm and Kubernetes versions),
- steps to reproduce or a proof of concept.

## What to expect

- Acknowledgement of your report within a few working days.
- An assessment of severity and affected versions.
- A coordinated fix and release, with credit to the reporter unless anonymity is
  requested.

## Hardening notes

helm-spray invokes the `helm` binary (using the `HELM_BIN` provided by the host
helm) and talks to the Kubernetes API directly through the embedded client — it
does not shell out to `kubectl`. It builds all command arguments programmatically
(never via a shell), keeps fetched charts inside a private temporary directory,
and redacts `--set`/`--set-string`/`--set-file` values from debug logs. Note that
`--debug` additionally prints the raw helm output (including rendered manifests),
which can contain sensitive values, so enable it only when needed.
Dependencies are scanned with `govulncheck` and the code with `gosec` in CI.
