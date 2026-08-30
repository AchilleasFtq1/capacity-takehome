package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
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
	srv.SetErrorPresenter(graph.PresentError)

	http.Handle("/", playground.Handler("capacity", "/query"))
	http.Handle("/query", callerFromHeader(srv))

	port := config.Port()
	log.Printf("playground  http://localhost:%s", port)
	log.Printf("graphql     http://localhost:%s/query", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
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
