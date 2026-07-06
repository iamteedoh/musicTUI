# Security Policy

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, use GitHub's private vulnerability reporting:

1. Go to the repository's **Security** tab.
2. Click **Report a vulnerability**.
3. Fill in the details.

This opens a private advisory visible only to you and the maintainers. We aim to
acknowledge reports within a few days.

If private reporting is unavailable, you may contact the maintainer directly —
see the profile at https://github.com/iamteedoh.

## What to include

- A description of the issue and its impact
- Steps to reproduce (or a proof of concept)
- Affected version / commit
- Any suggested remediation

## Scope

musicTUI runs entirely on your own machine and talks directly to Spotify,
Google/YouTube, and lyric providers using **your own OAuth applications**. Areas
that are most security-relevant:

- Handling and on-disk storage of OAuth tokens (encrypted at rest under your
  config directory)
- The local OAuth loopback/callback servers used during login and import
- Any place user-supplied credentials (client IDs/secrets) are read or stored

Please **never include real client secrets, tokens, or credentials** in a
report — redact them.

## Supported versions

This is an actively developed project; fixes land on `main` and ship in the next
tagged release. Please test against the latest release or `main` before
reporting.
