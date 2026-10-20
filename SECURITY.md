# Security policy

## Supported versions

tdeemon is in early development. Security fixes go into the next
release only, so upgrade to the [latest release][releases] before you
report.

## Reporting a vulnerability

Please do not open a public issue for a vulnerability. Report it
privately on GitHub instead: [report a vulnerability][report], also
reachable from the repository's security advisories page.

Include:

- what the problem is and what an attacker could do with it;
- the steps to reproduce it;
- the version, from `/api/v1/info` or the image tag.

Leave real personal data and secrets out of the report: made-up data is
enough to show the problem.

## What to expect

- An answer within 7 days.
- The report stays private, and so does the fix until it is ready to
  release; you can follow and discuss it in the private advisory.
- Then the advisory is published, crediting you unless you would rather
  not be named, and a CVE is requested where one applies.

tdeemon has one maintainer, so a fix comes as soon as it can, the most
serious first.

[releases]: https://github.com/zarnautovic/tdeemon/releases/latest
[report]: https://github.com/zarnautovic/tdeemon/security/advisories/new
