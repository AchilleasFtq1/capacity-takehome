package graph

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/tktaofik/capacity-takehome/api/graph/model"
	"github.com/tktaofik/capacity-takehome/api/internal/capacity"
	"github.com/tktaofik/capacity-takehome/api/internal/store"
)

// The GraphQL enums and the domain enums are the same strings deliberately, so
// the conversion stays a cast rather than a switch that can drift.
func toModelTier(t capacity.Tier) model.Tier  { return model.Tier(t) }
func toDomainTier(t model.Tier) capacity.Tier { return capacity.Tier(t) }
func toModelUser(u store.User) *model.User    { return &model.User{ID: u.ID.Hex(), Name: u.Name} }
func toModelStatus(s store.RequestStatus) model.RequestStatus {
	return model.RequestStatus(s)
}

// missingUser stands in for a user a row points at but the users collection no
// longer has. The schema says User! and a nil here would fail the whole query,
// taking the rest of someone's contact list down with it.
func missingUser(id bson.ObjectID) *model.User {
	return &model.User{ID: id.Hex(), Name: "Unknown"}
}

// resolveUsers loads every user referenced by a set of rows in one query, so a
// list of ten contacts is two round trips and not eleven.
func (r *Resolver) resolveUsers(ctx context.Context, ids []bson.ObjectID) (map[bson.ObjectID]store.User, error) {
	return r.Store.UsersByID(ctx, ids)
}

func (r *Resolver) toModelContacts(ctx context.Context, docs []store.Contact) ([]model.Contact, error) {
	ids := make([]bson.ObjectID, 0, len(docs))
	for _, c := range docs {
		ids = append(ids, c.OtherID)
	}
	users, err := r.resolveUsers(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make([]model.Contact, 0, len(docs))
	for _, c := range docs {
		user := missingUser(c.OtherID)
		if u, ok := users[c.OtherID]; ok {
			user = toModelUser(u)
		}
		out = append(out, model.Contact{
			ID:        c.ID.Hex(),
			User:      user,
			Tier:      toModelTier(c.Tier),
			CreatedAt: c.CreatedAt,
		})
	}
	return out, nil
}

func (r *Resolver) toModelRequests(ctx context.Context, docs []store.Request) ([]model.Request, error) {
	ids := make([]bson.ObjectID, 0, len(docs)*2)
	for _, q := range docs {
		ids = append(ids, q.FromID, q.ToID)
	}
	users, err := r.resolveUsers(ctx, ids)
	if err != nil {
		return nil, err
	}

	pick := func(id bson.ObjectID) *model.User {
		if u, ok := users[id]; ok {
			return toModelUser(u)
		}
		return missingUser(id)
	}

	out := make([]model.Request, 0, len(docs))
	for _, q := range docs {
		out = append(out, model.Request{
			ID:        q.ID.Hex(),
			From:      pick(q.FromID),
			To:        pick(q.ToID),
			Tier:      toModelTier(q.Tier),
			Status:    toModelStatus(q.Status),
			CreatedAt: q.CreatedAt,
		})
	}
	return out, nil
}

// toModelRequest is the single-row case, for the mutations that return one.
func (r *Resolver) toModelRequest(ctx context.Context, doc store.Request) (*model.Request, error) {
	rows, err := r.toModelRequests(ctx, []store.Request{doc})
	if err != nil {
		return nil, err
	}
	return &rows[0], nil
}

func (r *Resolver) toModelContact(ctx context.Context, doc store.Contact) (*model.Contact, error) {
	rows, err := r.toModelContacts(ctx, []store.Contact{doc})
	if err != nil {
		return nil, err
	}
	return &rows[0], nil
}
