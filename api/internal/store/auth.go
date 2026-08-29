package store

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type ctxKey struct{}

// ErrNoUser means the request arrived without a usable X-User-Id header.
var ErrNoUser = errors.New("no caller: send an X-User-Id header")

// WithUser puts the caller on the context. There is no real auth in this
// exercise and there should not be - see BRIEF.md, out of scope.
func WithUser(ctx context.Context, id bson.ObjectID) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// CallerID reads the caller back off the context.
func CallerID(ctx context.Context) (bson.ObjectID, error) {
	id, ok := ctx.Value(ctxKey{}).(bson.ObjectID)
	if !ok || id.IsZero() {
		return bson.ObjectID{}, ErrNoUser
	}
	return id, nil
}
