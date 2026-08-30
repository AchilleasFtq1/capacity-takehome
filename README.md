# The Capacity Problem

A contact list with a hard ceiling. Three tiers with their own caps, sharing a
budget that is smaller than their sum — so the interesting question is never
"is this tier full", it is "which limit is doing the refusing, and can the
person read the answer".

## Run it

Needs Go 1.25+, Node 20+, and Docker running.

```bash
git clone <this repo> capacity && cd capacity
make check     # ~15s from cold. starts mongo, builds, vets, tests, type-checks
```

That is the whole setup. To use it:

```bash
make up        # mongo on :27117 (replica set, so transactions work)
make api       # graphql on :8080, seeds ten users on first boot
make mobile    # expo — press i for the iOS simulator, w for web
make test-rules  # just the four rules, verbose, with -race
```

Playground at <http://localhost:8080>. Copy any `id` from `{ users { id name } }`
and send it as `X-User-Id`. The client has a user switcher in the header, so you
can act as two people and watch a seat be contended for.

### Two changes to the Makefile, and why

`make check` now runs `make up` first, and installs the client's dependencies
before type-checking. Both exist to make *green on clone* actually true, not to
bend it — I verified by cloning this repo into an empty directory and running
`make check` with nothing else set up: green in 13 seconds, against a fresh
Mongo volume.

- **`check` depends on `up`.** Rule 4 can only be proven against a real replica
  set, and its test skips itself when Mongo is unreachable. Left alone, the rule
  most likely to be got wrong is also the one most likely to go unrun. The test
  still self-skips with a loud message rather than exploding, so nothing is
  load-bearing on Docker except the proof itself.
- **`check` runs `npm install --silent` first.** As shipped, `cd mobile && npx
  tsc --noEmit` on a fresh clone **exits 1** — there is no `node_modules`, so
  npx prints *"This is not the tsc command you are looking for"* and no
  TypeScript is checked at all. I hit this on my first run. So the client half
  of `check` was never green on clone; now it is.

For the original behaviour without Docker: `cd api && go test ./...`.

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
the transaction alone — it does not work, and *How I know rule 4 actually holds*
below shows it failing.

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

### How I know rule 4 actually holds

A green test that has never been shown to fail proves nothing, and rule 4 is
exactly the rule where a serial test passes happily while production quietly
oversells seats. So I built the wrong version on purpose and pointed the test at
it.

The wrong version is the tempting one: wrap the accept in a transaction and
trust it. That does not work. MongoDB transactions give **snapshot** isolation,
not serialisability, and snapshot isolation permits *write skew* — two accepts
each read "7 of 8" from their own snapshot, then each insert a **different**
contact document. WiredTiger detects conflicts per document, and these are
different documents, so there is nothing to detect. Both commit. The user lands
at 9 of 8 and no error is raised anywhere.

One line in `AcceptRequest`, transaction left completely intact:

```diff
- locked, err := s.Store.LockSeats(ctx, caller, req.FromID)
+ locked, err := s.Store.UsersByID(ctx, []bson.ObjectID{caller, req.FromID})
```

Then, against a budget of exactly one seat:

```bash
go test ./internal/store/ -run TestConcurrentAcceptsTakeOneSeat -count=8
```

```
--- FAIL: TestConcurrentAcceptsTakeOneSeat
    both accepts raced one free seat: 2 succeeded, 0 refused for capacity (want 1 and 1)
```

Eight failures out of eight — not a timing window I got lucky with; the
transaction never had a chance to help. Restoring `LockSeats` and re-running
with `-count=15`: fifteen consecutive passes. It reproduces in about thirty
seconds if you want to watch it fail.

## Where the agent got it wrong

Small, real, and mine. The interesting part is not the bug, it is that nothing
in the toolchain was ever going to catch it.

### The claim

While writing the client's data hook, the agent needed a failed background
refresh to leave the stale list on screen and raise a banner, rather than blank
the page. It produced this, and presented it as the fix:

```js
setState((s) => {
  if (s.snapshot) setNotice({ kind: 'fault', text: message });   // ← wrong
  return { snapshot: s.snapshot, loading: false, fatal: s.snapshot ? null : message };
});
```

The implicit claim is that a `setState` updater is a reasonable place to read
current state *and* fire a second state update off the back of it. It reads
sensibly, it solves the stated problem, and it is wrong: React updaters must be
pure, and React reserves the right to invoke them more than once — under
StrictMode it deliberately double-invokes, which would have produced duplicate
banners.

### How I caught it

Re-reading my own diff before committing. That is the honest answer, and it is
the uncomfortable part: **nothing automated caught this.** It compiles. `npx
tsc --noEmit` passes clean — the types are all correct, because the bug is in
*when* the function runs, not in what it takes or returns. There is no test in
`mobile/` that would have failed. Every gate in `make check` is green on the
broken version.

So the only reason it isn't in the submission is that I read the code again
after the tool told me it was done. That is not a process I would want to rely
on twice.

### The result

Replaced with a ref (`snapshotRef`) that the callbacks read directly, so the
notice is fired from the async handler where side effects belong. Fixing it
surfaced a second bug I had not noticed: `refresh` had been taking
`state.snapshot` as a `useCallback` dependency, so its identity churned on every
render and the `useEffect` that depends on it would have refetched far more
often than intended. One bad instinct, two bugs, zero of them caught by a
machine.

Two smaller ones, both self-inflicted: I hand-rolled an `itoa` instead of
reaching for `strconv`, and I named a `Resolver` field `Contacts` where it
silently shadowed the generated `Contacts()` resolver method. That last one the
compiler caught instantly — which is exactly the contrast: the Go mistake was
free to find, and the React mistake cost a careful read.

*(The rule-4 falsification is not in this section on purpose. I did not get that
one wrong — I reasoned to the seat lock first and then built the naive version
deliberately to try to break my own test. It is written up under [How I know
rule 4 actually holds](#how-i-know-rule-4-actually-holds) as evidence, which is
what it is.)*

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

## Try the refusal

The shortest path to a refusal, entirely in the client: act as **You**, accept
requests from the inbox until the budget reads **8 / 8**, then accept the Pink
request still waiting. Pink shows **0 / 1** — visibly empty — and the refusal
says:

> You're using 8 of your 8 contact seats. Remove someone before you add anyone new.

Then move a Blue contact into that empty Pink tier. At 8 of 8 it works, because
re-filing spends nothing. That pair of actions is the whole exercise in two taps.
