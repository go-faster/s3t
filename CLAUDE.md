# CLAUDE.md

Guidance for working in this repository.

## What this is

`s3t` is a Go port of [ceph/s3-tests][upstream], the S3 compatibility suite, as a
standalone CLI binary rather than a pytest suite. The design, scope decisions, and
phase plan live in [PLAN.md](PLAN.md) — read it before making structural changes.

The upstream commit being tracked is pinned in [UPSTREAM](UPSTREAM).

## Keep the code minimal

**This is the primary rule.** Prefer the smallest thing that works:

- No abstraction until there are at least two real callers. No interface with one
  implementation, no options struct with one option, no helper wrapping one call.
- No configurability that nobody asked for. Flags, hooks, and knobs are added when a
  test or a user needs them, not in anticipation.
- No dependency that replaces a few lines of stdlib.
- Delete code rather than commenting it out or leaving it unreachable.
- If a function reads as obvious, it does not need a comment saying what it does.
  Comment *why*, and only where the reason is not on the page.

The suite is ~590 ported tests over a small harness. The harness stays small; the
tests are where the volume belongs.

## Conventions

- **No global variables.** No package-level mutable state, no `init()` registration.
  Pass values explicitly; return them from constructors. Test packages return their
  tests as slices, and the runner collects them.
- **Errors: `github.com/go-faster/errors`.** Not `fmt.Errorf`, not stdlib `errors.New`,
  not `pkg/errors`. `errors.Wrap(nil, ...)` returns a *non-nil* error, so only wrap
  inside an `if err != nil`. Wrap messages name the operation: `errors.Wrap(err,
  "decode config")`, never `"failed to decode config"`.
- **Test names match upstream exactly.** A ported test's `Name` is the Python function
  name minus the `test_` prefix, and `Module` is the upstream file. This is what lets
  results be joined against a pytest run and lets allow-list node IDs resolve. Renaming
  a test breaks the gate in `go-faster/fs`.
- **Markers are metadata, not behavior.** `fails_on_aws` and friends carry over for
  selection only; never make a test skip itself based on one.
- Ported test bodies should mirror the Python's structure and assertions closely enough
  that a reviewer can diff them against upstream. Idiomatic Go, same behavior.

## Commands

```console
make build      # go build -o s3t ./cmd/s3t
make test       # go test -race ./...
make test_fast  # go test ./...
make lint       # golangci-lint run ./...
```

Before committing: `gofmt -l .`, `go vet ./...`, `make lint`, `make test`.

## Commits

Conventional commits, enforced by commitlint in CI: `feat(harness): ...`,
`fix(client): ...`, `chore: ...`, `refactor(...): ...`, `docs: ...`.

Commit in small, self-contained steps — one change per commit, with its tests. Avoid
`git add -A` when unrelated work is in the tree; stage the files the commit is about.

The body explains *why*, not what the diff already shows.

## License

MIT, inherited from upstream, whose copyright notice is retained in
[LICENSE](LICENSE) and attribution in [NOTICE](NOTICE). New files need no header.

[upstream]: https://github.com/ceph/s3-tests
