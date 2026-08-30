#!/usr/bin/env python3
"""Demo state for the capacity app. Standard library only, no install step.

    python3 scripts/demo.py reset     put the database in the pre-demo state
    python3 scripts/demo.py dryrun    walk the whole demo, asserting each step
    python3 scripts/demo.py state     print who holds what

The demo is three taps and it proves all four rules:

    1. accept the Green request   -> the budget goes to 8 of 8
    2. accept the Pink request    -> REFUSED, even though Pink shows 0 of 1
    3. move a Blue contact to Pink -> ALLOWED at 8 of 8, because re-filing
                                      spends no seat

`reset` leaves the database at step 0 so the sequence always cooperates.
`dryrun` performs all three against the real API and fails loudly if any of
them stops behaving, which is the point: run it before the call, not during.
"""

import json
import sys
import urllib.error
import urllib.request

API = "http://localhost:8080/query"

# The arithmetic matters and is easy to get wrong: the demo needs the budget to
# land exactly on 8 of 8 with Pink still empty, so the refusal cannot be blamed
# on Pink. Blue fills to its cap of 3, Green holds 4 and takes a 5th on the
# first tap: 3 + 5 = 8, the whole budget, without ever touching Pink.
HERO = "You"
BLUES = ["Grace", "Alan", "Katherine"]  # 3, Blue's cap
GREENS = ["Barbara", "Edsger", "Radia", "Ada"]  # 4, one short of Green's cap
FILLS_THE_BUDGET = "Margaret"  # the Green request that takes You from 7 to 8 of 8
WANTS_THE_PINK_SEAT = "Ken"  # the Pink request that must then be refused


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


def reset():
    people = everyone()
    hero = people[HERO]
    wipe(people)

    for name in BLUES:
        connect(people[name], hero, "BLUE")
    for name in GREENS:
        connect(people[name], hero, "GREEN")

    # Two requests left waiting: one that fits, one that must be refused.
    call(
        people[FILLS_THE_BUDGET],
        "mutation($t:ID!){ sendRequest(toUserId:$t,tier:GREEN){ id } }",
        t=hero,
    )
    call(
        people[WANTS_THE_PINK_SEAT],
        "mutation($t:ID!){ sendRequest(toUserId:$t,tier:PINK){ id } }",
        t=hero,
    )

    print(f"{HERO} is now at  {describe(hero)}")
    print(f"Inbox: {FILLS_THE_BUDGET} (Green, fits), {WANTS_THE_PINK_SEAT} (Pink, must be refused)")
    return people, hero


def inbox_request_from(hero, sender_name):
    for request in call(hero, "{ incomingRequests { id tier from { name } } }")["incomingRequests"]:
        if request["from"]["name"] == sender_name:
            return request
    sys.exit(f"No pending request from {sender_name}. Run `reset` first.")


def dryrun():
    people, hero = reset()
    print()

    print("STEP 1  accept the Green request")
    call(
        hero,
        "mutation($r:ID!){ acceptRequest(requestId:$r){ tier } }",
        r=inbox_request_from(hero, FILLS_THE_BUDGET)["id"],
    )
    after = capacity(hero)
    print(f"        {describe(hero)}")
    assert after["budgetUsed"] == after["budgetCap"], "step 1 did not fill the budget"
    pink = next(t for t in after["tiers"] if t["tier"] == "PINK")
    assert pink["used"] == 0, "Pink should still be empty and visibly so"
    print("        budget full, Pink still empty. This is the setup for the refusal.")
    print()

    print("STEP 2  accept the Pink request  (the refusal)")
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
        sys.exit("STEP 2 FAILED: the Pink accept was allowed. Rule 1 is broken.")
    print("        Pink is 0 of 1 and still refused. Rule 1: the budget answers first.")
    print()

    print("STEP 3  move a Blue contact into that empty Pink tier")
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

    print("All three steps behaved. Re-running reset so the demo is armed.")
    reset()


def main():
    command = sys.argv[1] if len(sys.argv) > 1 else "state"
    if command == "reset":
        reset()
    elif command == "dryrun":
        dryrun()
    elif command == "state":
        print(describe(everyone()[HERO]))
    else:
        sys.exit(__doc__)


if __name__ == "__main__":
    main()
