# Security Policy

## Supported Versions

Security fixes are released for the latest published version of `dx` and the `main`
branch. Please upgrade to the latest release before reporting an issue that may
already be fixed.

## Reporting a Vulnerability

Please do not open a public GitHub issue for security vulnerabilities.

Report suspected vulnerabilities through GitHub's private vulnerability reporting
for this repository. If that is unavailable, email security@deductive.ai with:

- A concise description of the issue and its impact.
- Steps to reproduce or a proof of concept.
- Affected versions, operating systems, and relevant configuration.
- Any logs, screenshots, or traces that help verify the issue.

We will acknowledge reports within 3 business days and provide updates as we
investigate. Confirmed vulnerabilities will be fixed in a supported release and
documented in the release notes when disclosure is appropriate.

## Code Scanning

This repository uses GitHub CodeQL code scanning for Go. Findings appear in the
repository's Security tab and should be triaged before release.
