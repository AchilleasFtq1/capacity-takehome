package graph

import (
	"github.com/tktaofik/capacity-takehome/api/internal/capacity"
	"github.com/tktaofik/capacity-takehome/api/internal/store"
)

// Resolver is the root. Hold dependencies here; resolvers stay thin and push
// decisions down into the capacity package.
type Resolver struct {
	Store *store.Store
	Caps  capacity.Caps
}
