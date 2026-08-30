package graph

import (
	"context"
	"errors"
	"log"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/tktaofik/capacity-takehome/api/internal/capacity"
	"github.com/tktaofik/capacity-takehome/api/internal/contacts"
	"github.com/tktaofik/capacity-takehome/api/internal/store"
)

// PresentError decides what a caller is allowed to read.
//
// A refusal is a considered answer and goes out verbatim - the brief asks for
// "why" in plain words, and that sentence is built where the numbers are known.
// Anything else is a fault on our side: it gets logged in full and replaced,
// because a Mongo error string is neither useful to the user nor ours to leak.
//
// This lives in the graph package rather than in main so that a test can build
// the same server the binary does. An error presenter that only exists inside
// func main is an error presenter nothing can check.
func PresentError(ctx context.Context, e error) *gqlerror.Error {
	err := graphql.DefaultErrorPresenter(ctx, e)

	if refusal, ok := contacts.IsRefusal(e); ok {
		err.Message = refusal.Message
		err.Extensions = map[string]any{"code": CodeFor(refusal)}
		return err
	}
	if errors.Is(e, store.ErrNoUser) {
		err.Message = "No caller: send an X-User-Id header, or pick who you're acting as."
		err.Extensions = map[string]any{"code": "NO_CALLER"}
		return err
	}
	// Validation and parse failures arrive already shaped, and their message is
	// about the query the client sent, not about our internals.
	var already *gqlerror.Error
	if errors.As(e, &already) {
		return err
	}

	log.Printf("unexpected error at %v: %v", graphql.GetPath(ctx), e)
	err.Message = "Something went wrong on our side. Try again."
	err.Extensions = map[string]any{"code": "INTERNAL"}
	return err
}

// CodeFor gives the client something stable to branch on, so it never has to
// pattern-match on the prose.
func CodeFor(r *contacts.Refusal) string {
	switch {
	case errors.Is(r, capacity.ErrBudgetFull):
		return "BUDGET_FULL"
	case errors.Is(r, capacity.ErrTierFull):
		return "TIER_FULL"
	case errors.Is(r, contacts.ErrNotFound):
		return "NOT_FOUND"
	case errors.Is(r, contacts.ErrConflict):
		return "CONFLICT"
	default:
		return "INVALID"
	}
}
