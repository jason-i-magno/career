# career

A local-first job-search cockpit for engineers, written in Go.

Three things a job search actually needs, in one CLI:

- **A pipeline** that tells you what has gone quiet, not just what you applied to.
- **A STAR story bank** with a sanitization flag — for engineers whose best work
  is under NDA, proprietary, or classified.
- **A spaced-repetition deck** so interview preparation compounds instead of
  being crammed the week before.

```
$ career today

career  Sun 30 Aug 2026

Due now (1)
───────────
  #3  Confluent  tech  system design prep (today)

Gone quiet (2)
──────────────
  untouched for over two weeks — follow up or close them out
  #7  Grafana Labs   screen   18d ago
  #9  Redpanda       applied  21d ago

Prep due (23)
─────────────
  cpp 8/15   go 6/12   distsys 5/11   sysdes 4/8
  `career prep review`

3 live opportunities, 4 leads not yet applied to
```

## Install

```sh
go install github.com/jason-i-magno/career/cmd/career@latest
```

Or from a clone:

```sh
make install     # into $GOBIN, or ~/go/bin
make build       # to ./bin/career
```

Requires Go 1.25+. No cgo, so it cross-compiles to Linux, macOS and Windows
from any host.

## Quick start

```sh
career seed                          # 68 interview prep cards, 9 story stubs
career today                         # the morning dashboard

career app add Cloudflare "Systems Engineer" -track systems -source referral
career app set 1 -stage screen -next "prep system design" -due +3d
career app note 1 "Recruiter mentioned the team is hiring two seniors."
career app show 1

career story add "The hardest bug I have tracked down"
career story gaps                    # which competencies you cannot answer yet

career prep review                   # work through what is due
career stats                         # funnel and conversion by source
```

Run `career <command> -h` for any command's flags.

## Why it is shaped this way

**Stale beats sent.** Applications die from silence more often than rejection,
so `career today` surfaces anything untouched for two weeks rather than
celebrating your application count.

**Source is a first-class field.** Referrals convert several times better than
cold applications. Recording the channel up front is the only way to learn your
own rates — `career stats` breaks the funnel down by source, so you can stop
doing what is not working.

**Stories carry a sanitization flag.** If your best work is under NDA or a
clearance, the hard part is not remembering the story but telling it safely. A
story counts as interview-ready only when it is both complete *and* marked
sanitized — a deliberate human assertion, never inferred, because the version
you rehearse is the version that comes out under pressure.

**The funnel credits stages reached, not final outcome.** An application
rejected after an onsite still counts toward screen, tech and onsite. Being
rejected in the final round and being ignored after applying are opposite
problems with opposite fixes, and a funnel that conflates them tells you
nothing.

**The prep deck is opinionated.** `career seed` loads 68 cards across C++, Go,
distributed systems, cloud, system design and behaviour — weighted toward
low-latency systems and backend interviews. Prompts are phrased as questions you
answer out loud, because recognition feels like knowledge and is not. Delete
what does not apply to you and add your own.

## Architecture

```
cmd/career          entry point
internal/model      domain types and rules — no I/O, no SQL
internal/prep       SM-2 spaced repetition, a pure function of (state, grade, now)
internal/store      SQLite persistence, migrations, queries
internal/cli        command dispatch, flag parsing, rendering
```

The domain layer knows nothing about storage or presentation. That is what lets
the SM-2 scheduler be tested as pure arithmetic, and the store be tested against
a real temp-file database rather than a mock.

**Storage** is SQLite via [modernc.org/sqlite](https://modernc.org/sqlite), a
pure-Go implementation — no cgo, so builds stay simple and cross-compilation
works. Timestamps are RFC3339 text in UTC, which keeps the database readable
with the `sqlite3` CLI when something looks wrong. Migrations run on open,
tracked by SQLite's `user_version`.

Data lives at `~/.local/share/career/career.db`. Override with `CAREER_DB`.
Nothing leaves your machine.

## Development

```sh
make check       # lint + test, what CI runs
make test
make race
make cover
make help        # every target
```

## Notes

Flags may appear anywhere, including after positional arguments. Go's `flag`
package stops at the first non-flag argument, so `internal/cli/flags.go`
permutes them first — without it, `career app add Acme "Staff Engineer" -track
systems` would silently fold the flags into the role name.

`career seed` is idempotent: it matches on normalized prompt and title text, so
re-running after an upgrade adds only what is new and never disturbs your
scheduler state.

## Licence

MIT
