package capacity_test

import (
	"errors"
	"testing"

	"github.com/tktaofik/capacity-takehome/api/internal/capacity"
)

// testCaps mirrors the README defaults: sub-caps sum to 9, budget is 8.
func testCaps() capacity.Caps {
	return capacity.Caps{
		Budget: 8,
		PerTier: map[capacity.Tier]int{
			capacity.Pink:  1,
			capacity.Blue:  3,
			capacity.Green: 5,
		},
	}
}

// assertErr fails unless got is want, naming both. Sentinel identity is the
// whole point of these tests: "it returned an error" is not the assertion,
// "it returned the budget error and not the tier error" is.
func assertErr(t *testing.T, got, want error, context string) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("%s: got %v, want %v", context, got, want)
	}
}

func assertOK(t *testing.T, got error, context string) {
	t.Helper()
	if got != nil {
		t.Fatalf("%s: got %v, want nil", context, got)
	}
}

// Rule 1 - the shared budget binds before the sub-cap.
// 3 in Blue and 5 in Green is 8 of 8, so Pink is unreachable even though
// Pink is empty and its cap is 1.
func TestBudgetBindsBeforeSubCap(t *testing.T) {
	caps := testCaps()
	full := capacity.Counts{capacity.Blue: 3, capacity.Green: 5}

	if full.Total() != caps.Budget {
		t.Fatalf("setup: total %d, want %d", full.Total(), caps.Budget)
	}
	// Pink is empty and has room under its own cap. The budget still wins.
	if full[capacity.Pink] >= caps.PerTier[capacity.Pink] {
		t.Fatal("setup: Pink was meant to have room under its sub-cap")
	}

	assertErr(t, capacity.CanAdd(caps, full, capacity.Pink), capacity.ErrBudgetFull,
		"adding to an empty Pink at 8 of 8")

	// The other two tiers are full on both counts; the budget error is still the
	// one that comes back, because the sum is checked first.
	assertErr(t, capacity.CanAdd(caps, full, capacity.Blue), capacity.ErrBudgetFull,
		"adding to a full Blue at 8 of 8")
	assertErr(t, capacity.CanAdd(caps, full, capacity.Green), capacity.ErrBudgetFull,
		"adding to a full Green at 8 of 8")

	// One seat back and Pink opens up, which proves the refusal above was the
	// budget talking and not something wrong with Pink.
	freed := capacity.Counts{capacity.Blue: 2, capacity.Green: 5}
	assertOK(t, capacity.CanAdd(caps, freed, capacity.Pink), "adding to Pink at 7 of 8")
}

// Rule 1b - a full tier fails even when the budget has room.
func TestTierFullWithBudgetRemaining(t *testing.T) {
	caps := testCaps()
	have := capacity.Counts{capacity.Pink: 1}

	if have.Total() >= caps.Budget {
		t.Fatal("setup: the budget was meant to have room")
	}

	assertErr(t, capacity.CanAdd(caps, have, capacity.Pink), capacity.ErrTierFull,
		"adding to a full Pink with 7 seats of budget left")

	// Same counts, a tier with room: allowed. So the refusal really was the
	// sub-cap and not a stray budget check.
	assertOK(t, capacity.CanAdd(caps, have, capacity.Blue), "adding to an empty Blue")
	assertOK(t, capacity.CanAdd(caps, have, capacity.Green), "adding to an empty Green")
}

// Rule 2 - a pending request spends no seat, so sending is a budget question
// only, and one free seat permits any number of outstanding requests.
func TestSendChecksBudgetOnly(t *testing.T) {
	caps := testCaps()

	// Exactly one seat free, and it is the last one.
	oneFree := capacity.Counts{capacity.Pink: 1, capacity.Blue: 3, capacity.Green: 3}
	if got, want := oneFree.Total(), caps.Budget-1; got != want {
		t.Fatalf("setup: total %d, want %d", got, want)
	}

	// One free seat buys unlimited outstanding requests: sending does not
	// consume anything, so the same counts answer the same way every time.
	for i := range 50 {
		if err := capacity.CanSend(caps, oneFree); err != nil {
			t.Fatalf("send %d of 50 on one free seat: got %v, want nil", i+1, err)
		}
	}

	// Full tiers are not the send question. Pink is at 1 of 1 here and a Pink
	// request is still permitted - it is the accept that will have to answer for
	// the tier, and only if the tier is still full by then.
	assertOK(t, capacity.CanSend(caps, capacity.Counts{capacity.Pink: 1}),
		"sending while Pink is full but the budget has room")

	// No free seat at all, and sending stops.
	full := capacity.Counts{capacity.Blue: 3, capacity.Green: 5}
	assertErr(t, capacity.CanSend(caps, full), capacity.ErrBudgetFull, "sending at 8 of 8")
}

// Rule 3 - re-filing checks the destination sub-cap and never the budget,
// because the contact is already inside the budget.
func TestMoveIgnoresBudget(t *testing.T) {
	caps := testCaps()

	// Exactly full on the budget, but Blue has a seat spare.
	full := capacity.Counts{capacity.Pink: 1, capacity.Blue: 2, capacity.Green: 5}
	if got, want := full.Total(), caps.Budget; got != want {
		t.Fatalf("setup: total %d, want %d", got, want)
	}

	// The contrast that makes this test worth having: in this exact state an
	// add is refused for the budget, and a move is allowed.
	assertErr(t, capacity.CanAdd(caps, full, capacity.Blue), capacity.ErrBudgetFull,
		"adding to Blue at 8 of 8")
	assertOK(t, capacity.CanMove(caps, full, capacity.Green, capacity.Blue),
		"moving Green to Blue at 8 of 8")

	// The destination sub-cap is still enforced.
	blueFull := capacity.Counts{capacity.Pink: 1, capacity.Blue: 3, capacity.Green: 4}
	assertErr(t, capacity.CanMove(caps, blueFull, capacity.Green, capacity.Blue),
		capacity.ErrTierFull, "moving Green to a full Blue")

	// Moving into the tier the contact already sits in is a no-op, not a
	// refusal, even when that tier is at its cap and counting the contact itself.
	assertOK(t, capacity.CanMove(caps, blueFull, capacity.Blue, capacity.Blue),
		"moving Blue to Blue while Blue is full")
}

// used may legally exceed cap (a lowered cap, a merge). Nothing may assume
// used <= cap, and an over-budget user must fail closed rather than panic.
func TestOverBudgetIsHandled(t *testing.T) {
	caps := testCaps()

	// Eleven seats against a budget of 8, and Blue at 5 against a cap of 3.
	over := capacity.Counts{capacity.Pink: 0, capacity.Blue: 5, capacity.Green: 6}
	if over.Total() <= caps.Budget {
		t.Fatalf("setup: total %d was meant to exceed the budget %d", over.Total(), caps.Budget)
	}

	// Fail closed on everything that would spend another seat.
	assertErr(t, capacity.CanSend(caps, over), capacity.ErrBudgetFull, "sending while over budget")
	assertErr(t, capacity.CanAdd(caps, over, capacity.Pink), capacity.ErrBudgetFull,
		"adding to an empty Pink while over budget")

	// Re-filing still works, because it spends nothing. Someone who is over
	// budget after a lowered cap can still tidy up, which is the only way back.
	assertOK(t, capacity.CanMove(caps, over, capacity.Green, capacity.Pink),
		"moving Green to an empty Pink while over budget")
	assertErr(t, capacity.CanMove(caps, over, capacity.Green, capacity.Blue),
		capacity.ErrTierFull, "moving Green to an over-full Blue")

	// Nil and zero values are the other half of "do not panic": an empty Counts
	// is a real state (a new user) and a zero Caps is a misconfiguration that
	// must refuse rather than crash.
	assertOK(t, capacity.CanAdd(caps, nil, capacity.Pink), "adding for a user with no contacts")
	assertErr(t, capacity.CanSend(capacity.Caps{}, nil), capacity.ErrBudgetFull,
		"sending under a zero-value Caps")
	assertErr(t, capacity.CanAdd(capacity.Caps{}, nil, capacity.Pink), capacity.ErrUnknownTier,
		"adding under a zero-value Caps")
}

// Caps are configuration, so the enforcement path has to move when the numbers
// move. A hardcoded 8 or 5 anywhere in capacity.go fails this.
func TestCapsAreConfiguration(t *testing.T) {
	raised := capacity.Caps{
		Budget: 500,
		PerTier: map[capacity.Tier]int{
			capacity.Pink:  1,
			capacity.Blue:  3,
			capacity.Green: 500,
		},
	}
	have := capacity.Counts{capacity.Green: 400}

	assertOK(t, capacity.CanSend(raised, have), "sending with CAP_GREEN raised")
	assertOK(t, capacity.CanAdd(raised, have, capacity.Green), "adding with CAP_GREEN raised")
	// The default caps would have refused both of the above outright.
	assertErr(t, capacity.CanAdd(testCaps(), have, capacity.Green), capacity.ErrBudgetFull,
		"the same counts under the default caps")
}

// A tier with no configured ceiling is not a tier with a ceiling of zero.
// Nothing may be filed into a tier we have no cap for.
func TestUnknownTierFailsClosed(t *testing.T) {
	caps := testCaps()
	const ghost = capacity.Tier("PURPLE")

	assertErr(t, capacity.CanAdd(caps, nil, ghost), capacity.ErrUnknownTier, "adding to an unknown tier")
	assertErr(t, capacity.CanMove(caps, nil, capacity.Green, ghost), capacity.ErrUnknownTier,
		"moving to an unknown tier")
	assertErr(t, capacity.CanMove(caps, nil, ghost, capacity.Green), capacity.ErrUnknownTier,
		"moving from an unknown tier")
}
