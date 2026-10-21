# Contributing

Thanks for your interest. tdeemon is in early development: the design and
the plans still move, so open an issue to talk about anything bigger than
a small fix before you start on it.

Report a security vulnerability privately, as [SECURITY.md](SECURITY.md)
describes, never in a public issue.

## Licence of contributions

tdeemon is under the [AGPL-3.0-only](LICENSE), and so is every
contribution: by opening a pull request you release your change under the
same licence and confirm you have the right to do so, as the
[GitHub Terms of Service](https://docs.github.com/en/site-policy/github-terms/github-terms-of-service#6-contributions-under-repository-license)
also say. There is no Contributor License Agreement to sign.

## Development

The tools are pinned in `mise.toml`; install them with
[mise](https://mise.jdx.dev):

```sh
mise install       # Go, Node and every linter
lefthook install   # git hooks, once per clone
make check         # what CI checks, except a few that only CI runs
```

Without mise activated in the shell, run the tools as
`mise exec -- <tool>`, such as `mise exec -- lefthook install`.

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
