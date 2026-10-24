# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`career` — a local-first job-search CLI in Go: an application pipeline, a STAR
story bank, and an SM-2 spaced-repetition interview deck over SQLite.

This is a **public repository**. Keep it that way: no personal details, no
employer names, no résumé content, no real application data. The prep deck and
story stubs are written in the second person about nobody in particular, and
must stay that way.

## Environment

Go is installed at `/usr/local/go` (1.24.1) but is **not on `PATH` by default**.
It is symlinked into `~/.local/bin`, which `.bashrc` already adds. If `go` is
not found in a fresh shell:

```sh
ln -sf /usr/local/go/bin/go /usr/local/go/bin/gofmt ~/.local/bin/
```

`go.mod` declares Go 1.25 and `GOTOOLCHAIN=auto` fetches it automatically, so
the older `GOROOT` is not a problem. Known wart: `go test -cover` on a package
with no test files reports `no such tool "covdata"` from the auto-downloaded
toolchain. Harmless.

The machine has **podman, not docker**. `gh` is installed and authenticated.

## Commands

```sh
make check          # lint + test — run this before saying anything is done
make test           # go test ./...
make race           # tests under the race detector
make cover          # per-package coverage
make build          # ./bin/career
make lint           # gofmt check (fails on unformatted) + go vet
make doctor         # diagnose a missing Go toolchain
make install-service  # go install, then run `career serve` as a systemd user unit
```

Single test or package:

```sh
go test ./internal/prep/ -run TestReviewLapseResetsRepsAndPenalisesEase -v
go test ./internal/store/ -v
```

CI runs gofmt, vet, `go test -race -cover`, and a cross-compile matrix for
linux/darwin/windows. **The cross-compile job is load bearing** — it is what
guarantees the no-cgo property stays true. Do not introduce a cgo dependency
without discussing it.

## Architecture

```
cmd/career          entry point; delegates immediately to internal/cli
internal/model      domain types and rules — no I/O, no SQL, no printing
internal/prep       SM-2 scheduler: pure function of (item, grade, now)
internal/store      SQLite persistence, migrations, queries
internal/cli        dispatch, flag parsing, terminal rendering
internal/web        `career serve`: HTML pages over the same store and model
contrib/systemd     user unit for `career serve`; `@BIN@` is filled in at install
```

The layering is the point and is worth preserving:

- **`model` imports nothing from the other packages.** Business rules
  (`Story.Ready()`, `Application.Stale()`, `Triage`, `CoverageGaps`, stage ordering) live
  here so they can be tested without a database. New domain rules go here, not
  in `cli`.
- **`prep.Review` does not mutate its input and takes `now` as an argument.**
  That is what makes the scheduler testable as arithmetic. Keep it that way;
  never call `time.Now()` inside it.
- **`store` returns domain types**, never raw rows, and wraps every error with
  context. Lookups that miss return `ErrNotFound` (check with `errors.Is`).
- **`cli` and `web` own all presentation.** `model`, `prep` and `store` never
  print or render. CLI output goes to `Env.Out`/`Env.Err`, never `os.Stdout`
  directly, so commands stay testable. `web` renders embedded `html/template`
  pages and logs through the `slog.Logger` it is handed.

### Things that will bite you

**Flag permutation.** Go's `flag` package stops parsing at the first non-flag
argument, so `app add Acme "Staff Engineer" -track systems` would fold the flags
into the role name. Every command parses through `parseFlags`
(`internal/cli/flags.go`), which permutes flags ahead of positionals. **Use
`parseFlags(fs, args)`, never `fs.Parse(args)`.** There is a test for this.

**Partial updates.** `app set` applies only the flags the user actually passed,
via `fs.Visit`. Do not "simplify" it into assigning every field — that would
silently clear anything not specified.

**Migrations are append-only.** `store.migrations` is a slice applied in order
and tracked by SQLite's `user_version`. **Never edit an applied migration**; add
a new element. Editing one leaves existing databases silently inconsistent.

**Timestamps are RFC3339 text in UTC.** SQLite has no time type. Use the
`fmtTime`/`parseTime` helpers in `internal/store/store.go`, and remember
comparisons are lexical (which RFC3339 in UTC makes correct).

**Days are local, though times are stored in UTC.** Due dates, review scheduling
and labels like "today" or "in 3d" count calendar days in now's zone, through
`model.StartOfDay` and `model.DaysUntil`. Never `Truncate(24 * time.Hour)` a
time to find its day: that cuts at midnight UTC, which west of Greenwich falls
in the afternoon or evening.

**Foreign keys are enabled per-connection** via the DSN pragma in `store.Open`.
They are off by default in SQLite, and the cascade delete of events depends on
them. There is a test asserting the cascade works.

**`Sanitized` is a safety property, not a status field.** A story is
interview-ready only when complete *and* sanitized. Never default it to true,
never infer it, and do not add a command that flips it in bulk — the whole point
is that a human asserts each one deliberately.

**`career serve` has no login.** It listens on loopback and relies on a proxy
such as `tailscale serve` for TLS and access control. In its place, each with a
test: `checkHost` refuses foreign Host headers (DNS rebinding), and
`http.CrossOriginProtection` refuses cross-site form posts. Keep both, and
think about both before adding any endpoint that writes. A grade is applied only
while the card is due, so a resubmitted form cannot review it twice.
Unsanitized story text is deliberately never served; the CLI is the only place
to read it.

**The systemd unit stays unsandboxed apart from `NoNewPrivileges`.** In a user
service, `ProtectSystem`, `PrivateTmp` and the like need unprivileged user
namespaces, which some distributions restrict, and the unit then fails to start.
`serve` exits 0 on SIGTERM, so `systemctl --user stop` is not a failure and
`Restart=on-failure` leaves it stopped.

**`career seed` must stay idempotent.** It matches existing cards and stories on
whitespace/case-normalized text (`normalise`). Re-running after adding seed
content should add only what is new and never reset scheduler state.

**The seed deck is public content.** `internal/cli/seeddata.go` must stay
generic — no employer names, no personal history, no résumé specifics. Cards are
questions anyone in a systems or backend loop could be asked. The same applies to
test fixtures and README examples: use fictional companies (Acme, Globex) and
invented stories, never anything that reads as real application data.

## Conventions

- Errors wrap with `%w` and describe the operation: `fmt.Errorf("inserting
  application: %w", err)`. Lowercase, no trailing punctuation.
- Table-driven tests with subtests. Test names state the behaviour, not the
  function (`TestReviewLapseResetsRepsAndPenalisesEase`).
- Store tests use a real SQLite file under `t.TempDir()`, not mocks.
- Comments explain *why*. Several encode domain reasoning (why the funnel
  credits stages reached, why sanitization gates readiness) — do not strip them
  as noise.
- British/American spelling is mixed in prose; not worth normalising.
