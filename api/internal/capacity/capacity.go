// Package capacity holds the tier rules.
//
// It is pure on purpose: no database, no context, no clock, no IO. Everything
// a decision needs is passed in, which is what makes the rules cheap to test
// and impossible to accidentally scatter across resolvers.
//
// The four rules these functions must satisfy are in the README. Read them there,
// not here.
package capacity

import "errors"

type Tier string

const (
	Pink  Tier = "PINK"
	Blue  Tier = "BLUE"
	Green Tier = "GREEN"
)

// Tiers lists every tier, closest first.
func Tiers() []Tier { return []Tier{Pink, Blue, Green} }

// Label is the human name for a tier, for refusals that have to read as a
// sentence. It lives here because the name belongs to the tier, not to a screen.
func (t Tier) Label() string {
	switch t {
	case Pink:
		return "Pink flag"
	case Blue:
		return "Blue flag"
	case Green:
		return "Green flag"
	default:
		return string(t)
	}
}

// Caps is configuration, loaded at startup. Raising a cap must never require
// a code change in the enforcement path.
type Caps struct {
	Budget  int
	PerTier map[Tier]int
}

// Cap is the ceiling for one tier, and whether that tier is configured at all.
// A tier missing from PerTier is not a tier with a cap of zero, it is a tier we
// do not know about, and the two have to fail differently.
func (c Caps) Cap(t Tier) (int, bool) {
	n, ok := c.PerTier[t]
	return n, ok
}

// Counts is a snapshot of one user's active contacts, keyed by tier.
type Counts map[Tier]int

// Total is the number of seats currently spent across every tier.
func (c Counts) Total() int {
	n := 0
	for _, t := range Tiers() {
		n += c[t]
	}
	return n
}

var (
	// ErrBudgetFull means the shared budget is spent, regardless of sub-caps.
	ErrBudgetFull = errors.New("capacity: shared budget is full")
	// ErrTierFull means the destination tier is full, even though the budget has room.
	ErrTierFull = errors.New("capacity: tier is full")
	// ErrUnknownTier means the tier is not in the configured caps. Nothing may be
	// filed into a tier we have no ceiling for, so this fails closed.
	ErrUnknownTier = errors.New("capacity: unknown tier")
)

// CanSend reports whether a user holding these counts may send a new request.
// A pending request creates no contact and spends no seat.
//
// Rule 2: this is a budget question only. The destination tier is deliberately
// not checked - the tier may well be full now and empty by the time anyone
// accepts, and refusing here would be guessing at a future we cannot see. It is
// also stateless with respect to outstanding requests, which is what lets one
// free seat carry any number of them.
func CanSend(caps Caps, have Counts) error {
	if have.Total() >= caps.Budget {
		return ErrBudgetFull
	}
	return nil
}

// CanAdd reports whether a new contact may be added to tier t.
// Called for both sides of an accept.
//
// Rule 1: the budget is checked before the sub-cap, so a user at 8 of 8 is
// refused for the budget even when the named tier is empty.
func CanAdd(caps Caps, have Counts, t Tier) error {
	limit, known := caps.Cap(t)
	if !known {
		return ErrUnknownTier
	}
	if have.Total() >= caps.Budget {
		return ErrBudgetFull
	}
	if have[t] >= limit {
		return ErrTierFull
	}
	return nil
}

// CanMove reports whether an existing contact may be re-filed from one tier
// to another. The contact already occupies a seat.
//
// Rule 3: the destination sub-cap is checked and the budget is not. The seat is
// already spent and re-filing does not spend another, so a budget check here
// would block a legal move for a user who is exactly full - which is precisely
// the user most likely to want to reorganise.
func CanMove(caps Caps, have Counts, from, to Tier) error {
	if _, known := caps.Cap(from); !known {
		return ErrUnknownTier
	}
	limit, known := caps.Cap(to)
	if !known {
		return ErrUnknownTier
	}
	// A move to the tier the contact is already in changes nothing. Without this
	// it would be refused whenever that tier is at its cap, since the contact is
	// counted against the ceiling it is being measured for.
	if from == to {
		return nil
	}
	if have[to] >= limit {
		return ErrTierFull
	}
	return nil
}
