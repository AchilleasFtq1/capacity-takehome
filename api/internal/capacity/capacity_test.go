package capacity_test

import (
	"testing"

	"github.com/tktaofik/capacity-takehome/api/internal/capacity"
)

// testCaps mirrors the defaults in BRIEF.md: sub-caps sum to 9, budget is 8.
func testCaps() capacity.Caps {
	return capacity.Caps{
		Budget: 8,
		PerTier: map[capacity.Tier]int{
			capacity.Partner: 1,
			capacity.Crew:    3,
			capacity.Circle:  5,
		},
	}
}

// Rule 1 - the shared budget binds before the sub-cap.
// 3 in Crew and 5 in Circle is 8 of 8, so Partner is unreachable even though
// Partner is empty and its cap is 1.
func TestBudgetBindsBeforeSubCap(t *testing.T) {
	t.Skip("delete this line and write the test")
	_ = testCaps()
}

// Rule 1b - a full tier fails even when the budget has room.
func TestTierFullWithBudgetRemaining(t *testing.T) {
	t.Skip("delete this line and write the test")
}

// Rule 2 - a pending request spends no seat, so sending is a budget question
// only, and one free seat permits any number of outstanding requests.
func TestSendChecksBudgetOnly(t *testing.T) {
	t.Skip("delete this line and write the test")
}

// Rule 3 - re-filing checks the destination sub-cap and never the budget,
// because the contact is already inside the budget.
func TestMoveIgnoresBudget(t *testing.T) {
	t.Skip("delete this line and write the test")
}

// used may legally exceed cap (a lowered cap, a merge). Nothing may assume
// used <= cap, and an over-budget user must fail closed rather than panic.
func TestOverBudgetIsHandled(t *testing.T) {
	t.Skip("delete this line and write the test")
}
