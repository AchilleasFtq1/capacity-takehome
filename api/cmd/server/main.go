package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/tktaofik/capacity-takehome/api/graph"
	"github.com/tktaofik/capacity-takehome/api/internal/config"
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

	srv := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{
		Resolvers: &graph.Resolver{Store: st, Caps: config.Load()},
	}))

	http.Handle("/", playground.Handler("capacity", "/query"))
	http.Handle("/query", callerFromHeader(srv))

	port := config.Port()
	log.Printf("playground  http://localhost:%s", port)
	log.Printf("graphql     http://localhost:%s/query", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
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
