package contacts

import (
	"errors"
	"fmt"

	"github.com/tktaofik/capacity-takehome/api/internal/capacity"
)

// Refusal is an error a user is allowed to read.
//
// Everything else that can go wrong here - a dropped connection, a decode
// failure - is ours, not theirs, and gets a generic sentence instead. The
// split is what lets the API surface "Ada's Pink flag tier is full (1 of 1)"
// without also leaking a Mongo error string the day something breaks.
type Refusal struct {
	// Message is plain words, addressed to the person who hit it.
	Message string
	// Reason is the sentinel underneath, kept so callers and tests can ask what
	// kind of refusal this was without matching on prose.
	Reason error
}

func (r *Refusal) Error() string { return r.Message }
func (r *Refusal) Unwrap() error { return r.Reason }

// Sentinels for the refusals that are not about capacity.
var (
	ErrNotFound = errors.New("contacts: not found")
	ErrConflict = errors.New("contacts: conflicting state")
	ErrInvalid  = errors.New("contacts: invalid input")
)

func refuse(reason error, format string, args ...any) *Refusal {
	return &Refusal{Message: fmt.Sprintf(format, args...), Reason: reason}
}

// IsRefusal reports whether err is safe to show to a user, and returns it.
func IsRefusal(err error) (*Refusal, bool) {
	var r *Refusal
	ok := errors.As(err, &r)
	return r, ok
}

// holder is whichever side of an operation ran out of room. Refusals name the
// person, because "capacity: shared budget is full" tells the user nothing
// about whose budget or what to do next.
type holder struct {
	Name string
	Self bool
}

func self() holder             { return holder{Self: true} }
func other(name string) holder { return holder{Name: name} }

// refuseCapacity turns a sentinel from the capacity package into a sentence.
// The rule decided; this only explains the decision.
// Your own numbers are yours, so a refusal about your list is specific and
// actionable. Someone else's are not: an accept that fails because the *sender*
// is full must say so without publishing how full, or which tier, or how close
// to a cap they are. Anyone who can send a request could otherwise probe a
// stranger's list one message at a time, and "Ada's Pink flag tier is full
// (1 of 1)" is a fact about Ada that Ada never agreed to share.
//
// The accepter loses nothing they could act on - they cannot free somebody
// else's seat - so this costs no actionability at all.
func refuseCapacity(h holder, t capacity.Tier, caps capacity.Caps, have capacity.Counts, reason error) *Refusal {
	if !h.Self && (errors.Is(reason, capacity.ErrBudgetFull) || errors.Is(reason, capacity.ErrTierFull)) {
		return refuse(reason, "%s can't take another contact right now, so this can't go through.", h.Name)
	}

	switch {
	case errors.Is(reason, capacity.ErrBudgetFull):
		return refuse(reason,
			"You're using %d of your %d contact seats. Remove someone before you add anyone new.",
			have.Total(), caps.Budget)

	case errors.Is(reason, capacity.ErrTierFull):
		limit, _ := caps.Cap(t)
		return refuse(reason,
			"Your %s tier is full (%d of %d). Move someone out of it, or choose another tier.",
			t.Label(), have[t], limit)

	case errors.Is(reason, capacity.ErrUnknownTier):
		return refuse(ErrInvalid, "%q isn't a tier this app has a limit for.", string(t))
	}

	// A new sentinel in capacity with no sentence here. Say something true and
	// unhelpful rather than something specific and wrong.
	return refuse(reason, "That isn't allowed right now.")
}
