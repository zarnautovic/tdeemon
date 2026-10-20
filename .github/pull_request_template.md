## What and why

<!-- What changes, and why; link the issue if there is one. Replace the
comments: this description becomes the body of the squash commit. -->

## How it was checked

<!-- The tests, `make check`, and anything checked by hand. -->

## Definition of Done

- [ ] Changes in behaviour are covered by tests that fail without them
- [ ] `make check` and CI pass
- [ ] Generated files are fresh: the OpenAPI spec and TypeScript types,
      the schema snapshot
- [ ] The docs say what changed: README, `docs/conventions.md`,
      `docs/DESIGN.md`
- [ ] No personal data and no secrets in the change
- [ ] The title is a Conventional Commit (`feat: …`, `fix(store): …`)

<!-- A breaking change takes `!` in the title, or a last line here that
reads "BREAKING CHANGE: what breaks" (docs/conventions.md). -->
