# Contributing

Thanks for your interest. tdeemon is in early development: the design and
the plans still move, so open an issue to talk about anything bigger than
a small fix before you start on it.

## The Contributor License Agreement

Contributions need a signed Contributor License Agreement. It is being set
up and will be linked here before the first outside contribution is
accepted.

## Development

The tools are pinned in `mise.toml`:

```sh
mise install       # Go, Node and every linter
lefthook install   # git hooks
make check         # everything CI checks
```

The rules for code, tests, commits and pull requests are in
[docs/conventions.md](docs/conventions.md); coding agents read
[AGENTS.md](AGENTS.md).

## Pull requests

- The title is a Conventional Commit, such as `feat: …` or
  `fix(store): …`; it becomes the commit when the pull request is
  squash-merged.
- CI must pass, and the template's Definition of Done must hold.
- Keep personal data and secrets out of the change: the repository is
  public.
