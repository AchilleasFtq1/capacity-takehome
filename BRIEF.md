# The Capacity Problem

Build a small social app where the contact list has a hard ceiling. The features
are simple. The rules underneath them are not — that's the exercise.

**6–8 hours, one day.** Use AI agents freely; we do. Ends with a 45-minute call
where you demo it and make one live change.

## Stack

Already scaffolded and running. Clone, `make up`, `make api`, go.

| | |
|---|---|
| API | Go 1.25 · GraphQL (gqlgen) · MongoDB 8 |
| Client | Expo SDK 57 · React Native · TypeScript |
| Local | Docker — Mongo on `:27117`, API on `:8080` |

Never written Go or shipped an Expo app? Not a disqualifier — getting productive
in an unfamiliar stack inside a day is part of what we're measuring. If mobile
genuinely blocks you, a web client is fine; say so in the README.

## Tiers

| Tier | Cap |
|---|---|
| Partner | 1 |
| Crew | 3 |
| Circle | 5 |
| **Shared budget** | **8** |

Sub-caps sum to 9. The budget is 8. That gap is deliberate.

## The four rules

Most of the grade is here, not in the screens.

**1. Budget before sub-cap.** 3 Crew + 5 Circle is 8 of 8. That person cannot
add a Partner, even though Partner is empty. The sum is checked first.

**2. A pending request holds no seat.** Sending creates no contact. One free
seat buys unlimited outstanding requests. Capacity is checked at **accept**,
against **both** people — either side being full fails it.

**3. Re-filing is not adding.** Moving Circle → Crew checks the destination
sub-cap only, never the budget. The contact is already inside the budget; a
budget check here blocks a legal move.

**4. Two people can want the last seat.** Concurrent accepts on one free seat:
exactly one wins, the other fails cleanly. Read-then-write is not an answer.

Also: `used` may legally exceed `cap` (a lowered cap, a merge). Fail closed,
don't panic.

Caps are config, not constants — already done for you in `api/internal/config`.
Don't undo it.

## Build

| | | |
|---|---|---|
| **R1** | Send a request to a named tier. | core |
| **R2** | Accept / decline. Accept creates the contact on both sides. | core |
| **R3** | Move a contact between tiers. | core |
| **R4** | Remove a contact. Frees the seat on both sides. | core |
| **R5** | People screen: contacts by tier, live `used / cap`, budget visible. | core |
| **R6** | Request inbox. A failed accept says *why*, in plain words. | core |
| **R7** | Posts scoped to a tier and everything closer. | stretch |
| **R8** | Optimistic accept with rollback. | stretch |

**Out of scope, don't spend time here:** auth (`X-User-Id` is wired), signup,
profiles, search, push, deployment, visual polish.

## Hand in

- Repo plus a README that runs.
- **Decisions** — 3 or 4 calls you made and what you rejected. We read this first.
- **What's next, and what's unfinished.** Unfinished with a reason beats hand-waved.
- **Where the agent got it wrong** — one thing your AI tooling got confidently
  wrong and how you caught it. Can't name one? We'll assume you didn't check.
- Tests for the rules. Stubs are in place; delete the `t.Skip` lines.

## The call — 45 minutes

- **10 min** — you demo it, including one refusal.
- **20 min** — two features in depth. Why this shape, what you rejected, where it
  breaks first.
- **15 min** — one live change to your own code, on the call, with your normal tools.

## Graded on

| | |
|---|---|
| Rules correct | All four hold, proven by a test. |
| Where logic lives | One pure, testable rule — not scattered across resolvers and screens. |
| Failure surface | A refusal reaches the user as a sentence they understand. |
| Scope judgment | You cut the right things and said so. |
| Command of your code | You can change it and explain it live. Outweighs the rest. |

Questions before you start are free. Ambiguity you resolved and wrote down beats
a question you didn't ask.
