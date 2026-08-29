# The Capacity Problem

Build a small social app where your contact list has a hard ceiling. The
features are simple on purpose. The rules underneath them are not, and that is
the whole exercise.

- **Budget:** 6–8 hours. One day, wall clock.
- **AI agents:** expected, not merely tolerated. Use whatever you normally use.
- **Ends with:** a 45-minute call where you demo it and make one live change.

---

## 1. What you're building

A relationship-tiered contact app. Everyone you know sits in exactly one tier,
every tier is capped, and the caps are the product — scarcity is what makes the
app mean something.

| Tier | Sub-cap | Meaning |
|---|---|---|
| Partner | 1 | The one person |
| Crew | 3 | Closest few |
| Circle | 5 | People you keep up with |
| **Shared budget** | **8** | **Across all three tiers, combined** |

The sub-caps add up to 9. The budget is 8. That gap is deliberate.

---

## 2. The rules of capacity

Read these twice. Most of the grade lives here, not in the screens.

**Rule 1 — the budget binds before the sub-cap.**
A person with 3 in Crew and 5 in Circle is at 8 of 8. They **cannot** add a
Partner, even though Partner is empty and its cap is 1. The sum is checked
first, always.

**Rule 2 — a pending request occupies nothing.**
Sending a request creates no contact and consumes no seat. One free seat buys
unlimited outstanding requests. Capacity is checked when a request is
**accepted**, and it is checked against **both** people — either side being full
fails the accept.

**Rule 3 — re-filing is not adding.**
Moving an existing contact from Circle to Crew checks the **destination sub-cap
only**. It never checks the budget, because that contact is already inside the
budget. A budget check here wrongly blocks a legal move.

**Rule 4 — two people can want the last seat.**
Two accepts landing at the same moment on a user with one free seat must not
both succeed. Exactly one wins; the other gets a clean, readable failure.
Read-then-write is not an answer.

**And one design constraint.** Caps are configuration, never compile-time
constants. The litmus test: raising Circle from 5 to 500 must be a config change
and nothing else. This part is already done for you in
`api/internal/config` — don't undo it by hardcoding a cap in the enforcement
path. Partner is the one structural exception; 1 is an invariant there and you
may enforce it as one.

One more: `used` may legally exceed `cap` (a lowered cap, a merge). Nothing may
assume `used <= cap`, and an over-budget user must fail closed rather than
panic.

---

## 3. What to build

| | Requirement | |
|---|---|---|
| **R1** | **Send a request.** User A asks User B to join a named tier. No capacity gate beyond the sender's budget having at least one free seat. | core |
| **R2** | **Accept or decline.** Accepting creates the contact on both sides and enforces Rules 1, 2 and 4. Declining is quiet — no contact, no seat. | core |
| **R3** | **Move a contact between tiers.** Enforces Rule 3. | core |
| **R4** | **Remove a contact.** Frees the seat on both sides at once. It is one relationship, not two. | core |
| **R5** | **People screen.** Contacts grouped by tier, each with a live `used / cap` count, and the shared budget shown somewhere honest. | core |
| **R6** | **Request inbox.** Incoming requests with accept and decline. When an accept fails on capacity, the user must see *why* in plain words — not a toast that says "error". | core |
| **R7** | **Tier-scoped posts.** A post addressed to a tier is visible to that tier and everything closer. Only if the core is genuinely done. | stretch |
| **R8** | **Optimistic accept with rollback.** The row moves immediately and snaps back correctly when the server refuses. | stretch |

### Explicitly out of scope — do not spend time here

- Real auth. `X-User-Id` is already wired; the client already has a user switcher.
- Signup, profiles, avatars, settings, push, search.
- Deployment. It runs on your machine.
- Visual polish beyond legible. We are not grading the design.

---

## 4. What to hand in

- A repo we can clone, with a **README that runs** — exact commands, nothing assumed.
- A **decisions section**: the calls you made, and the option you rejected for
  each. Three or four is plenty. This is the part we read first.
- A **"what I'd do next"** list, and an honest note on anything unfinished.
  Unfinished with a reason scores better than finished and hand-waved.
- A section on **where the agent got it wrong** — a place your AI tooling
  produced something confidently incorrect, how you caught it, and what you
  changed. If you can't name one, we'll assume you didn't check.
- Tests for the capacity rules. Not the whole app — the rules. Stubs are already
  in place at `api/internal/capacity/capacity_test.go` and
  `api/internal/store/race_test.go`; delete the `t.Skip` lines and fill them in.

---

## 5. The demo call

45 minutes, you sharing your screen. Nothing to prepare beyond having it runnable.

- **You drive it — 10 min.** Show it working, including at least one refusal: a
  state where the app correctly says no.
- **Two features, in depth — 20 min.** We pick them. Expect: why this shape,
  what you rejected, where it breaks first under load, and which parts you wrote
  versus reviewed.
- **One live change — 15 min.** A small modification to your own code, on the
  call, with whatever tools you normally use. We're watching how you work, not
  whether you finish.

---

## 6. How we read it

| We look at | What good looks like |
|---|---|
| **Rule correctness** | All four rules hold, including the race. Proven by a test, not by claim. |
| **Where logic lives** | The capacity rule is one pure, testable thing — not scattered across resolvers and screens. |
| **Failure surface** | A refusal reaches the user as a sentence they understand. |
| **Judgment on scope** | You cut the right things and said so. Gold-plating a screen while Rule 4 is broken reads badly. |
| **Command of your own code** | On the call, you can change it and explain it. This one outweighs the rest. |

Use every AI tool you want, for as much of it as you want. We do. The only thing
we care about is whether you understood and verified what came out — which is
exactly what the live change on the call will show.

Questions before you start are welcome and cost you nothing. Ambiguity you
resolved yourself, written down in the README, scores better than a question you
didn't ask.
