# Contributing to Flixarr

Flixarr is in early development, so things move fast and change often. Bug reports and ideas are very welcome.

## Issues

- Search the open issues before filing a new one.
- One bug or one idea per issue.
- Use the Bug report or Feature request form. Security problems go through a [private report](https://github.com/Flixarr/flixarr/security/advisories/new), never a public issue.

## Branches

- `develop` is where work happens. Nightly builds come from it.
- `main` only moves when a release is published, through a pull request from `develop`.

## Pull requests

- Open pull requests against `develop`, not `main`.
- Keep each one to a single fix or feature, and link its issue (`Fixes #12`).
- The checks must pass: `gofmt`, `go vet`, `go test ./...`, and in `web/` `npm run check` and `npm run build`.

## Building

See the README for running it with Docker. The server is Go, the web app is SvelteKit in `web/`.

## License

By contributing you agree that your work is released under the project's [GPL 3.0 license](LICENSE).
