package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/tktaofik/capacity-takehome/api/graph"
	"github.com/tktaofik/capacity-takehome/api/internal/capacity"
	"github.com/tktaofik/capacity-takehome/api/internal/config"
	"github.com/tktaofik/capacity-takehome/api/internal/contacts"
	"github.com/tktaofik/capacity-takehome/api/internal/store"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	st, err := store.Connect(ctx, config.MongoURI())
	if err != nil {
		log.Fatalf("mongo: %v (is `make up` running?)", err)
	}
	if err := st.Seed(ctx); err != nil {
		log.Fatalf("seed: %v", err)
	}

	caps := config.Load()
	log.Printf("caps        budget %d, %s", caps.Budget, describeTiers(caps))

	srv := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{
		Resolvers: &graph.Resolver{
			Store:   st,
			Caps:    caps,
			Service: contacts.New(st, caps),
		},
	}))
	srv.SetErrorPresenter(presentError)

	http.Handle("/", playground.Handler("capacity", "/query"))
	http.Handle("/query", callerFromHeader(srv))

	port := config.Port()
	log.Printf("playground  http://localhost:%s", port)
	log.Printf("graphql     http://localhost:%s/query", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// presentError decides what a caller is allowed to read.
//
// A refusal is a considered answer and goes out verbatim - the brief asks for
// "why" in plain words, and that sentence is built where the numbers are known.
// Anything else is a fault on our side: it gets logged in full and replaced,
// because a Mongo error string is neither useful to the user nor ours to leak.
func presentError(ctx context.Context, e error) *gqlerror.Error {
	err := graphql.DefaultErrorPresenter(ctx, e)

	if refusal, ok := contacts.IsRefusal(e); ok {
		err.Message = refusal.Message
		err.Extensions = map[string]any{"code": codeFor(refusal)}
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

// codeFor gives the client something stable to branch on, so it never has to
// pattern-match on the prose.
func codeFor(r *contacts.Refusal) string {
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

// describeTiers logs the caps at boot, so `CAP_GREEN=500 make api` is visibly
// in effect without anyone having to query for it.
func describeTiers(caps capacity.Caps) string {
	parts := make([]string, 0, len(capacity.Tiers()))
	for _, t := range capacity.Tiers() {
		limit, _ := caps.Cap(t)
		parts = append(parts, fmt.Sprintf("%s %d", t, limit))
	}
	return strings.Join(parts, ", ")
}

// callerFromHeader stands in for authentication. Send X-User-Id with the id of
// whichever seeded user you are acting as; `query { users { id name } }` lists them.
func callerFromHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if raw := r.Header.Get("X-User-Id"); raw != "" {
			if id, err := bson.ObjectIDFromHex(raw); err == nil {
				r = r.WithContext(store.WithUser(r.Context(), id))
			}
		}
		next.ServeHTTP(w, r)
	})
}
