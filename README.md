# The Capacity Problem

A contact list with a hard ceiling. Three tiers with their own caps, sharing a
budget that is smaller than their sum — so the interesting question is never
"is this tier full", it is "which limit is doing the refusing, and can the
person read the answer".

## Run it

Needs Go 1.25+, Node 20+, Docker.

```bash
make up        # mongo on :27117 (replica set, so transactions work)
make api       # graphql on :8080, seeds ten users on first boot
make mobile    # expo — press i for the iOS simulator, w for web
make check     # build + vet + go test + tsc. now depends on `up` (see below)
make test-rules  # just the four rules, verbose, with -race
```

Playground at <http://localhost:8080>. Copy any `id` from `{ users { id name } }`
and send it as `X-User-Id`. The client has a user switcher in the header, so you
can act as two people and watch a seat contended for.

**One change to the harness:** `make check` now depends on `make up`. The rule 4
test needs a real replica set and skips itself when Mongo is unreachable, and
the rule most likely to be got wrong should not also be the one most likely to
go unrun. If you want the old behaviour, `cd api && go test ./...`.

## What's here

R1–R6 and R8. **R7 (tier-scoped posts) is not built** — see *What I dropped*.

| | | |
|---|---|---|
| R1 | Send a request to a named tier | ✅ |
| R2 | Accept / decline, contact on both sides | ✅ |
| R3 | Move a contact between tiers | ✅ |
| R4 | Remove, frees the seat on both sides | ✅ |
| R5 | People screen: tiers, live used/cap, budget | ✅ |
| R6 | Inbox, refusals in plain words | ✅ |
| R7 | Posts scoped to a tier and closer | ❌ dropped |
| R8 | Optimistic accept with rollback | ✅ |

## Where the rules live

```
api/internal/capacity/    the rules. pure: no db, no ctx, no clock, no IO
api/internal/contacts/    the only place the rules and the database meet
api/internal/store/       mongo: documents, indexes, transactions, the seat lock
api/graph/                resolvers. thin - read caller, call service, convert
mobile/src/useAppData.ts  one query, five mutations, the optimistic accept
```

The layering is the point. `capacity` decides and cannot reach a database;
`store` reads and writes and does not know a rule exists; `contacts` is the one
seam. A capacity check appearing in a resolver or inside a Mongo query would be
a second copy of a rule that is meant to have exactly one home.

## Decisions

**1. A service package between the resolvers and the store.** The scaffold's
`store` doc comment says business rules do not live there, and I wanted to keep
that true, but the accept flow is four database calls that have to be atomic
together — that orchestration needs a home. *Rejected:* putting the transaction
in the resolver (mixes GraphQL with locking, and untestable without an HTTP
layer), and putting it in `store` (contradicts the comment, and drags `Caps`
into the data layer). The cost is one more package to explain; the benefit is
that the race test drives the service directly, with no server running.

**2. Rule 4 is a per-user seat lock, not a counter.** Accept bumps a
`seatVersion` field on both user documents *before* reading either count. Two
concurrent accepts then write the same document, one gets a write conflict, the
driver retries it, and the retry re-reads counts the winner has already
committed. *Rejected:* a denormalised `seatsUsed` counter with a conditional
`$inc` — atomic, but it is a second source of truth that drifts from the actual
contact rows, and reconciling it is a job nobody wants. Also rejected: trusting
the transaction alone. See below, it does not work.

**3. Refusals are sentences, built where the numbers are.** `capacity` returns
sentinels; `contacts` turns a sentinel plus the caps, the counts and the
person's name into `"Ada's Pink flag tier is full (1 of 1)"`; an error presenter
passes refusals through verbatim and replaces everything else with a generic
line after logging it. *Rejected:* returning codes and letting the client write
the copy — that puts a third copy of the rules in the UI, in the one place
nobody tests, and it drifts the first time a cap changes.

**4. The client never pre-checks capacity.** No greyed-out tiers, no hidden
Accept buttons. Every destination is offered and the server refuses if it must.
*Rejected:* disabling what will not fit, which reads as polish and is really a
fourth copy of the rules — and it is wrong anyway, because a seat can free up
between render and tap. This is also why the refusal banner is loud: it is the
whole feedback mechanism, so it had better be legible.

**5. The accepter is checked before the sender.** Both sides are checked, but
the accepter's own refusal is the one they can act on, so it wins when both are
full.

## Ambiguities I resolved

The brief says a resolved-and-written-down ambiguity beats an unasked question.
These are the calls I made:

- **Both sides land in the tier the sender named.** `acceptRequest` takes no
  tier argument, so the schema had already decided this. It is a slightly odd
  social model — you cannot choose how you file someone who asked to be a Pink
  flag — but R3 exists, and the accepter can re-file immediately. That is also
  what makes R3 feel necessary rather than decorative.
- **Sending checks the budget and not the destination tier.** Rule 2 puts the
  capacity question at accept. A Pink request while Pink is full is allowed to
  be sent; it may well fit by the time it is answered.
- **Moving a contact into the tier it is already in is a no-op, not a refusal.**
  Otherwise it fails whenever that tier is at its cap, because the contact is
  being counted against the ceiling it is being measured for.
- **A tier with no configured cap fails closed** (`ErrUnknownTier`), rather than
  being treated as a cap of zero. Unreachable through GraphQL — the enum
  rejects it first — but it is the correct answer for a misconfigured
  `CAP_*` env var.
- **A pending request in the other direction blocks a new one**, so a pair can
  never have two live requests to resolve.
- **Declined requests stay visible to the sender** and leave the recipient's
  inbox. The sender is the only person who otherwise never learns what happened.

## Tests for the rules

```bash
make test-rules
```

`api/internal/capacity/capacity_test.go` — pure, instant, no database. Seven
tests: budget-before-sub-cap, tier-full-with-budget-left, send-checks-budget-
only, move-ignores-budget, over-budget-does-not-panic, caps-are-configuration,
unknown-tier-fails-closed. They assert *which* sentinel comes back, not merely
that something failed — "refused" is not the assertion, "refused for the budget
and not for the tier" is.

`api/internal/store/race_test.go` — six tests against a real replica set:
the concurrent accept, both-sides-checked, remove-frees-both-seats,
move-over-a-full-budget, duplicate-request-refused, pending-requests-hold-no-seat.

Rule 3 is asserted twice on purpose — once pure, once end to end — because
"never check the budget here" is the rule a later refactor is most likely to
tidy away.

## Where the agent got it wrong

**"It's in a transaction, so it's safe."** This is the confident wrong answer,
and it is what you get from most tooling and most of the internet. MongoDB
transactions give *snapshot* isolation, and snapshot isolation does not prevent
write skew: two accepts read "7 of 8" from their own snapshots and then insert
two *different* contact documents. Different documents means WiredTiger sees no
conflict, both commit, and the user ends up at 9 of 8 — with no error anywhere.

I didn't take my own reasoning on faith either. I replaced `LockSeats` with a
plain `UsersByID` read — a read-then-write inside a perfectly good transaction —
and ran the race test eight times:

```
--- FAIL: TestConcurrentAcceptsTakeOneSeat
    both accepts raced one free seat: 2 succeeded, 0 refused (want 1 and 1)
```

Every run, deterministically. Restored the lock: 15 consecutive passes. That
falsification is the only reason I'd claim rule 4 holds — a green test that has
never been shown to fail proves nothing, and this is exactly the rule where a
serial test passes and production loses seats.

**One I actually wrote and had to fix:** in `useAppData.ts` I called
`setNotice(...)` from inside a `setState` updater. It type-checked, it would
have worked most of the time, and it is wrong — updaters must be pure, React
may invoke them twice, and StrictMode would have produced duplicate banners. No
compiler or test caught it; I caught it re-reading the diff. It is now a ref
(`snapshotRef`), which also removed a stale-closure bug in `refresh` that I had
not noticed until I went looking. Smaller ones: a hand-rolled `itoa` instead of
`strconv`, and a `Resolver.Contacts` field that silently shadowed the generated
`Contacts()` resolver method until the compiler complained.

## What I dropped, and why

**R7, tier-scoped posts.** It is a whole second domain — a posts collection, a
visibility query joining against contact tiers, a composer, a feed — and doing
it in the time left would have meant a thin version of it *and* less certainty
about rule 4. The brief says R1–R6 is the core and that what you consciously
drop is part of what is being read, so: dropped on purpose, and the four rules
got the time instead.

Out of scope per the brief and deliberately untouched: auth, signup, profiles,
search, push, deployment, visual polish.

## What's next, and what's unfinished

- **No test at the GraphQL layer.** The service is well covered and the
  resolvers are thin pass-throughs, but "thin" is an assertion I made by
  reading, not by testing. I verified the whole surface by hand over HTTP
  (every mutation, every refusal, bad ids, missing header); that should be a
  Go test against `handler.NewDefaultServer`.
- **The seat lock is a hot document per user.** Correct, and fine at this scale,
  but every accept and removal touching a person serialises on one row. At real
  volume I would want to know how often the driver is retrying before defending
  this shape.
- **Declined requests accumulate.** Nothing prunes them; the outgoing list grows
  forever. A TTL index or a sender-side dismiss is the obvious fix.
- **No pagination, no live updates.** Ten seeded users, pull to refresh. Two
  simulators showing each other a contended seat need a manual refresh to agree.
- **`used > cap` is handled but not reachable through the UI.** The rules and
  the meter both cope (the bar clamps, the number does not, the refusal still
  reads properly). The only way to produce it today is to lower a `CAP_*` env
  var and restart — try `CAP_GREEN=1 make api` against a populated account.
- **The optimistic accept assumes one in flight at a time.** It captures and
  restores the whole snapshot, so two accepts tapped inside the same round trip
  would roll back to the older of the two. Correct for a human tapping a button;
  not correct in general.

## Try the refusal in ten seconds

```bash
# Fill a user to 3 Blue + 5 Green = 8 of 8, leaving Pink empty, then have
# someone ask for the empty Pink seat. Rule 1: the budget answers first.
```

The demo path in the client: act as **You**, accept from the inbox until the
budget reads 8/8, then accept the Pink request that is still waiting. Pink shows
0/1 — visibly empty — and the refusal says:

> You're using 8 of your 8 contact seats. Remove someone before you add anyone new.

Then move a Blue contact into that empty Pink tier. At 8 of 8 it works, because
re-filing spends nothing. That pair of actions is the whole exercise in two taps.
