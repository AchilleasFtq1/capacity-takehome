# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

A take-home exercise, now implemented. `README.md` is the submission write-up
(decisions, dropped scope, unfinished work); the original brief is in git
history at `823e0b5`. A social app where the contact list has a hard ceiling:
three tiers (PINK cap 1, BLUE cap 3, GREEN cap 5) sharing a budget of 8. The
sub-caps sum to 9 and the budget is 8 — that gap is what the whole exercise
turns on.

The graded criteria are: the four rules holding (proven by a test), the rule
living in one pure testable place, refusals reaching the user as a sentence, and
being able to explain and change the code live. Optimise for those, not for
feature count.

## Commands

```bash
make up        # mongo 8 on :27117 as a single-node replica set (transactions need one)
make api       # graphql on :8080, seeds ten users on first boot
make mobile    # expo dev server — i for iOS simulator, w for web
make generate  # regenerate gqlgen code after editing graph/schema.graphqls
make check     # build + vet + go test + tsc; depends on `up`
make test-rules  # the four rules only, verbose, with -race
make down      # stop mongo    /    make clean — stop and drop the volume
```

Single test:

```bash
cd api && go test ./internal/capacity/ -run TestMoveIgnoresBudget -v
cd api && go test ./internal/store/ -run TestConcurrentAcceptsTakeOneSeat -count=15   # needs `make up`
```

`make check` depends on `up` deliberately: the rule 4 test skips itself when
Mongo is unreachable, and that rule must not go silently unproven. Env: `CAP_BUDGET`,
`CAP_PINK`, `CAP_BLUE`, `CAP_GREEN`, `MONGO_URI`, `PORT`, and `EXPO_PUBLIC_API_URL`
on the client. `CAP_GREEN=500 make api` must work with no recompilation.

Mongo is on **27117**, not 27017, to avoid colliding with a local instance.

Note: Go is installed via Homebrew on this machine (`/opt/homebrew/bin`), which
is not always on `PATH` in a fresh non-login shell.

## Architecture

Three Go packages in a deliberate stack, plus a thin GraphQL layer:

- `api/internal/capacity/` — **the graded surface.** Pure: no database, no
  context, no clock, no IO. `CanSend` / `CanAdd` / `CanMove` take `Caps` and
  `Counts` and return `nil`, `ErrBudgetFull`, `ErrTierFull`, or `ErrUnknownTier`.
  Every capacity decision resolves here. Never add IO to this package, and never
  let a capacity check appear in a resolver or a Mongo query — that is a second
  copy of a rule that is meant to have one home.
- `api/internal/store/` — Mongo only: connection, document shapes, indexes,
  seed, `CountsFor`, `WithTransaction`, `LockSeats`. No business rules.
- `api/internal/contacts/` — the application service, and the only place the
  rules and the database meet. Owns the five mutations and the `Refusal` type.
- `api/graph/` — gqlgen. `generated.go` and `model/models_gen.go` are generated;
  edit `schema.graphqls` then `make generate`, never hand-edit them. Resolvers
  read the caller, call the service, convert. `convert.go` batch-loads users so
  a list of N rows is two queries, not N+1.
- `mobile/src/useAppData.ts` — one snapshot query, five mutations, and the
  optimistic accept with snapshot rollback (R8).

Beware: `Resolver` embeds into `queryResolver`, so a field named after a query
(e.g. `Contacts`) is shadowed by the generated method. The service field is
therefore called `Service`.

## The rules being graded

1. **Budget before sub-cap.** 3 BLUE + 5 GREEN = 8/8 blocks a PINK add even
   though PINK is empty. `CanAdd` checks the total first.
2. **A pending request holds no seat.** Sending checks the budget only, never
   the destination tier. Capacity is checked at **accept**, against **both**
   users — either side being full fails it.
3. **Re-filing is not adding.** `CanMove` checks the destination sub-cap and
   never the budget. Same-tier moves are a no-op, not a refusal.
4. **Two people can want the last seat.** `AcceptRequest` calls
   `store.LockSeats` (a `$inc` on each user doc) **before** reading any counts,
   inside a snapshot-isolated transaction. This is load-bearing: Mongo
   transactions alone do *not* prevent this — two accepts insert different
   documents, WiredTiger sees no conflict, and both commit. Verified by
   falsification: swapping `LockSeats` for a plain read makes
   `TestConcurrentAcceptsTakeOneSeat` fail 2-succeeded-of-2, every run.

Also: `used` may legally exceed `cap` (a lowered cap, a merge). Nothing may
assume `used <= cap`; fail closed rather than panic.

## Conventions

- Sentinel errors compared with `errors.Is`; wrap with `%w`.
- User-facing text is a `contacts.Refusal`, built where the caps, counts and
  names are all in scope. The error presenter in `cmd/server/main.go` passes
  refusals through verbatim and replaces everything else after logging it — so
  never put an internal detail in a `Refusal` message.
- The client never pre-checks capacity (no greyed-out tiers). The server
  refuses; the banner shows the sentence.
- Tier values are the uppercase strings `PINK`/`BLUE`/`GREEN` on the wire, in
  Mongo and in the domain; iterate with `capacity.Tiers()`, closest first.
- Client TypeScript is `strict`.

## Testing note

A race test that has never been shown to fail proves nothing. When changing
anything in the accept path, re-run the falsification: neuter `LockSeats`,
confirm `TestConcurrentAcceptsTakeOneSeat` fails, then restore.
