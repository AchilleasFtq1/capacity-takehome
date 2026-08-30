#!/usr/bin/env python3
"""Demo state for the capacity app. Standard library only, no install step.

    python3 scripts/demo.py reset             arm it: full at 8 of 8, Pink empty
    python3 scripts/demo.py reset --lead-in   arm it one seat short, at 7 of 8
    python3 scripts/demo.py dryrun            walk the taps, asserting each one
    python3 scripts/demo.py state             print who holds what

Default (`reset`) opens **already full**, so the refusal is the very first tap:

    1. accept the Pink request     -> REFUSED, though Pink shows 0 of 1
    2. move a Blue contact to Pink -> ALLOWED at 8 of 8, because re-filing
                                      spends no seat

Two taps, and the refusal lands inside the first minute. The screen carries the
argument on its own: budget 8/8, Blue 3/3, Green 5/5, and Pink the only tier
with room - which is exactly the tier that gets refused.

`--lead-in` starts a seat short with a Green request waiting, so tap one fills
the budget on camera before the refusal. Costs a tap, buys the transition.

`dryrun` performs every tap against the real API and fails loudly if any of them
stops behaving, which is the point: run it before the call, not during.
"""

import json
import sys
import urllib.error
import urllib.request

API = "http://localhost:8080/query"

# The arithmetic matters and is easy to get wrong. Blue at its cap of 3 plus
# Green at its cap of 5 is exactly the budget of 8, with Pink never touched -
# the only arrangement that fills the budget and leaves a visibly empty tier.
# In --lead-in mode Green holds one fewer and Margaret's request supplies it.
HERO = "You"
BLUES = ["Grace", "Alan", "Katherine"]  # 3, Blue's cap
GREENS = ["Barbara", "Edsger", "Radia", "Ada", "Margaret"]  # 5, Green's cap
LEAD_IN_SENDER = "Margaret"  # held back in --lead-in, sends the Green request
WANTS_THE_PINK_SEAT = "Ken"  # the Pink request that must be refused


class Refused(Exception):
    """The API refused, with a sentence meant for a person."""

    def __init__(self, message: str, code: str) -> None:
        super().__init__(message)
        self.message = message
        self.code = code


def call(user, query, **variables):
    body = json.dumps({"query": query, "variables": variables}).encode()
    request = urllib.request.Request(API, body, {"Content-Type": "application/json"})
    if user:
        request.add_header("X-User-Id", user)
    try:
        payload = json.loads(urllib.request.urlopen(request).read())
    except urllib.error.URLError as exc:
        sys.exit(f"Cannot reach the API at {API} ({exc}). Is `make api` running?")
    if payload.get("errors"):
        failure = payload["errors"][0]
        raise Refused(failure["message"], failure.get("extensions", {}).get("code", "?"))
    return payload["data"]


def everyone():
    return {u["name"]: u["id"] for u in call(None, "{ users { id name } }")["users"]}


def capacity(user):
    return call(user, "{ capacity { budgetUsed budgetCap tiers { tier used cap } } }")["capacity"]


def describe(user):
    c = capacity(user)
    tiers = "  ".join(f'{t["tier"].title()} {t["used"]}/{t["cap"]}' for t in c["tiers"])
    return f'{c["budgetUsed"]}/{c["budgetCap"]} budget   {tiers}'


def wipe(people):
    """Remove every contact and settle every pending request, for everyone."""
    for user in people.values():
        for contact in call(user, "{ contacts { id } }")["contacts"]:
            call(user, "mutation($c:ID!){ removeContact(contactId:$c) }", c=contact["id"])
    for user in people.values():
        for request in call(user, "{ incomingRequests { id } }")["incomingRequests"]:
            call(user, "mutation($r:ID!){ declineRequest(requestId:$r){ id } }", r=request["id"])


def connect(sender, receiver, tier):
    """Send and accept, so the pair ends up as contacts in `tier`."""
    sent = call(
        sender,
        "mutation($t:ID!,$tier:Tier!){ sendRequest(toUserId:$t,tier:$tier){ id } }",
        t=receiver,
        tier=tier,
    )
    call(
        receiver,
        "mutation($r:ID!){ acceptRequest(requestId:$r){ id } }",
        r=sent["sendRequest"]["id"],
    )


def reset(lead_in=False):
    """Arm the demo. Full at 8 of 8 by default; a seat short with --lead-in."""
    people = everyone()
    hero = people[HERO]
    wipe(people)

    greens = [n for n in GREENS if not (lead_in and n == LEAD_IN_SENDER)]
    for name in BLUES:
        connect(people[name], hero, "BLUE")
    for name in greens:
        connect(people[name], hero, "GREEN")

    if lead_in:
        # Held back so tap one fills the budget where everyone can see it.
        call(
            people[LEAD_IN_SENDER],
            "mutation($t:ID!){ sendRequest(toUserId:$t,tier:GREEN){ id } }",
            t=hero,
        )

    # The request that must be refused. Pink is empty; the budget answers first.
    call(
        people[WANTS_THE_PINK_SEAT],
        "mutation($t:ID!){ sendRequest(toUserId:$t,tier:PINK){ id } }",
        t=hero,
    )

    waiting = [f"{WANTS_THE_PINK_SEAT} (Pink, must be refused)"]
    if lead_in:
        waiting.insert(0, f"{LEAD_IN_SENDER} (Green, fits)")
    print(f"{HERO} is now at  {describe(hero)}")
    print("Inbox: " + ", ".join(waiting))
    if not lead_in:
        print("Tap one IS the refusal. Pink is the only tier with room.")
    return people, hero


def inbox_request_from(hero, sender_name):
    for request in call(hero, "{ incomingRequests { id tier from { name } } }")["incomingRequests"]:
        if request["from"]["name"] == sender_name:
            return request
    sys.exit(f"No pending request from {sender_name}. Run `reset` first.")


def dryrun(lead_in=False):
    people, hero = reset(lead_in)
    print()
    step = 0

    if lead_in:
        step += 1
        print(f"STEP {step}  accept the Green request")
        call(
            hero,
            "mutation($r:ID!){ acceptRequest(requestId:$r){ tier } }",
            r=inbox_request_from(hero, LEAD_IN_SENDER)["id"],
        )
        print(f"        {describe(hero)}")

    # However we got here, the board must read 8 of 8 with Pink empty, or the
    # refusal that follows could be blamed on the tier instead of the budget.
    board = capacity(hero)
    assert board["budgetUsed"] == board["budgetCap"], "the budget is not full"
    pink = next(t for t in board["tiers"] if t["tier"] == "PINK")
    assert pink["used"] == 0, "Pink must be visibly empty"
    print("        budget full, Pink empty. This is the setup for the refusal.")
    print()

    step += 1
    print(f"STEP {step}  accept the Pink request  (the refusal)")
    try:
        call(
            hero,
            "mutation($r:ID!){ acceptRequest(requestId:$r){ tier } }",
            r=inbox_request_from(hero, WANTS_THE_PINK_SEAT)["id"],
        )
    except Refused as refusal:
        print(f'        REFUSED [{refusal.code}]  "{refusal.message}"')
        assert refusal.code == "BUDGET_FULL", f"wrong reason: {refusal.code}"
        assert describe(hero).startswith("8/8"), "the refusal must change nothing"
    else:
        sys.exit(f"STEP {step} FAILED: the Pink accept was allowed. Rule 1 is broken.")
    print("        Pink is 0 of 1 and still refused. Rule 1: the budget answers first.")
    print()

    step += 1
    print(f"STEP {step}  move a Blue contact into that empty Pink tier")
    blue = next(c for c in call(hero, "{ contacts { id tier } }")["contacts"] if c["tier"] == "BLUE")
    moved = call(
        hero,
        "mutation($c:ID!){ moveContact(contactId:$c, tier:PINK){ tier user { name } } }",
        c=blue["id"],
    )["moveContact"]
    print(f'        moved {moved["user"]["name"]} to {moved["tier"]}')
    print(f"        {describe(hero)}")
    assert describe(hero).startswith("8/8"), "a move must not change the budget"
    print("        Allowed at 8 of 8. Rule 3: re-filing is not adding.")
    print()

    print(f"All {step} steps behaved. Re-arming so the demo is ready.")
    reset(lead_in)


def main():
    command = sys.argv[1] if len(sys.argv) > 1 else "state"
    lead_in = "--lead-in" in sys.argv[2:]
    if command == "reset":
        reset(lead_in)
    elif command == "dryrun":
        dryrun(lead_in)
    elif command == "state":
        print(describe(everyone()[HERO]))
    else:
        sys.exit(__doc__)


if __name__ == "__main__":
    main()
