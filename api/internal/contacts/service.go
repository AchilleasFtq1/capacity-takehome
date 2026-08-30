// Package contacts is the application layer: it sequences database work around
// the decisions made in package capacity.
//
// The split matters. capacity decides and knows nothing about Mongo; store
// reads and writes and knows nothing about the rules; this package is the only
// place the two meet. A capacity check that appears in a resolver or in a Mongo
// query is a bug, because it is a second copy of a rule that is supposed to
// have exactly one home.
package contacts

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/tktaofik/capacity-takehome/api/internal/capacity"
	"github.com/tktaofik/capacity-takehome/api/internal/store"
)

type Service struct {
	Store *store.Store
	Caps  capacity.Caps
}

func New(s *store.Store, caps capacity.Caps) *Service {
	return &Service{Store: s, Caps: caps}
}

// objectID parses a hex id from the client, refusing rather than 500ing on junk.
func objectID(hex, what string) (bson.ObjectID, error) {
	id, err := bson.ObjectIDFromHex(hex)
	if err != nil {
		return bson.ObjectID{}, refuse(ErrInvalid, "That %s id isn't valid.", what)
	}
	return id, nil
}

// SendRequest creates a pending request. It spends no seat: rule 2 says the
// capacity question is asked at accept, against both people, and asking it here
// as well would refuse requests that will be perfectly legal by the time anyone
// answers them.
func (s *Service) SendRequest(ctx context.Context, from bson.ObjectID, toHex string, tier capacity.Tier) (*store.Request, error) {
	to, err := objectID(toHex, "user")
	if err != nil {
		return nil, err
	}
	if to == from {
		return nil, refuse(ErrInvalid, "You can't send yourself a request.")
	}
	if _, known := s.Caps.Cap(tier); !known {
		return nil, refuse(ErrInvalid, "%q isn't a tier this app has a limit for.", string(tier))
	}

	var created store.Request
	err = s.Store.WithTransaction(ctx, func(ctx context.Context) error {
		// No seat lock here on purpose. Sending changes nobody's seat count, so
		// there is nothing to serialise, and taking the lock would make senders
		// contend with accepters for no benefit.
		var target store.User
		if err := s.Store.Users.FindOne(ctx, bson.M{"_id": to}).Decode(&target); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return refuse(ErrNotFound, "That person isn't here any more.")
			}
			return err
		}

		n, err := s.Store.Contacts.CountDocuments(ctx, bson.M{"ownerId": from, "otherId": to})
		if err != nil {
			return err
		}
		if n > 0 {
			return refuse(ErrConflict, "%s is already one of your contacts.", target.Name)
		}

		// A request the other way round already exists, so the answer is in the
		// inbox rather than in a second request. Blocking it here is also what
		// keeps a pair from ending up with two live requests to resolve.
		reverse, err := s.Store.Requests.CountDocuments(ctx,
			bson.M{"fromId": to, "toId": from, "status": store.RequestPending})
		if err != nil {
			return err
		}
		if reverse > 0 {
			return refuse(ErrConflict, "%s has already sent you a request. Answer it from your inbox.", target.Name)
		}

		counts, err := s.Store.CountsFor(ctx, from)
		if err != nil {
			return err
		}
		if err := capacity.CanSend(s.Caps, counts); err != nil {
			return refuse(err,
				"You're using %d of your %d contact seats, so you can't send new requests. Remove someone first.",
				counts.Total(), s.Caps.Budget)
		}

		created = store.Request{
			FromID:    from,
			ToID:      to,
			Tier:      tier,
			Status:    store.RequestPending,
			CreatedAt: time.Now().UTC(),
		}
		res, err := s.Store.Requests.InsertOne(ctx, created)
		if mongo.IsDuplicateKeyError(err) {
			return refuse(ErrConflict, "You already have a request waiting with %s.", target.Name)
		}
		if err != nil {
			return err
		}
		created.ID = res.InsertedID.(bson.ObjectID)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

// AcceptRequest turns a pending request into a contact on both sides.
//
// This is where every capacity rule actually bites, and where rule 4 is decided:
// both users are locked before either count is read, so two accepts racing for
// one free seat cannot both see it free.
func (s *Service) AcceptRequest(ctx context.Context, caller bson.ObjectID, requestHex string) (*store.Contact, error) {
	requestID, err := objectID(requestHex, "request")
	if err != nil {
		return nil, err
	}

	var accepted store.Contact
	err = s.Store.WithTransaction(ctx, func(ctx context.Context) error {
		var req store.Request
		err := s.Store.Requests.FindOne(ctx, bson.M{
			"_id":    requestID,
			"toId":   caller,
			"status": store.RequestPending,
		}).Decode(&req)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return refuse(ErrNotFound, "That request is no longer waiting for an answer.")
		}
		if err != nil {
			return err
		}

		// Lock first, count second. The other order is the read-then-write the
		// brief warns about: it passes every serial test and loses seats under
		// load.
		locked, err := s.Store.LockSeats(ctx, caller, req.FromID)
		if err != nil {
			if errors.Is(err, store.ErrUserNotFound) {
				return refuse(ErrNotFound, "That person isn't here any more.")
			}
			return err
		}
		sender := locked[req.FromID]

		// Already connected - a request left over from a pair that got joined
		// some other way. The outcome they asked for is already true, so settle
		// the request instead of leaving it stuck in the inbox for ever.
		existing := s.Store.Contacts.FindOne(ctx, bson.M{"ownerId": caller, "otherId": req.FromID})
		if err := existing.Decode(&accepted); err == nil {
			return s.settle(ctx, requestID)
		} else if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}

		mine, err := s.Store.CountsFor(ctx, caller)
		if err != nil {
			return err
		}
		theirs, err := s.Store.CountsFor(ctx, req.FromID)
		if err != nil {
			return err
		}

		// Either side being full fails the accept. The accepter is checked first
		// because that refusal is the one they can act on.
		if err := capacity.CanAdd(s.Caps, mine, req.Tier); err != nil {
			return refuseCapacity(self(), req.Tier, s.Caps, mine, err)
		}
		if err := capacity.CanAdd(s.Caps, theirs, req.Tier); err != nil {
			return refuseCapacity(other(sender.Name), req.Tier, s.Caps, theirs, err)
		}

		now := time.Now().UTC()
		accepted = store.Contact{OwnerID: caller, OtherID: req.FromID, Tier: req.Tier, CreatedAt: now}
		theirSide := store.Contact{OwnerID: req.FromID, OtherID: caller, Tier: req.Tier, CreatedAt: now}

		res, err := s.Store.Contacts.InsertMany(ctx, []any{accepted, theirSide})
		if mongo.IsDuplicateKeyError(err) {
			return refuse(ErrConflict, "You're already connected to %s.", sender.Name)
		}
		if err != nil {
			return err
		}
		accepted.ID = res.InsertedIDs[0].(bson.ObjectID)

		return s.settle(ctx, requestID)
	})
	if err != nil {
		return nil, err
	}
	return &accepted, nil
}

// settle marks a request accepted, and insists it was still pending when it
// did. A zero match means someone answered it inside this transaction's
// lifetime, which has to abort the accept rather than write a second contact.
func (s *Service) settle(ctx context.Context, requestID bson.ObjectID) error {
	res, err := s.Store.Requests.UpdateOne(ctx,
		bson.M{"_id": requestID, "status": store.RequestPending},
		bson.M{"$set": bson.M{"status": store.RequestAccepted}})
	if err != nil {
		return err
	}
	if res.MatchedCount != 1 {
		return refuse(ErrConflict, "That request was answered somewhere else. Pull to refresh.")
	}
	return nil
}

// DeclineRequest closes a request without creating anything.
func (s *Service) DeclineRequest(ctx context.Context, caller bson.ObjectID, requestHex string) (*store.Request, error) {
	requestID, err := objectID(requestHex, "request")
	if err != nil {
		return nil, err
	}

	var req store.Request
	err = s.Store.Requests.FindOneAndUpdate(ctx,
		bson.M{"_id": requestID, "toId": caller, "status": store.RequestPending},
		bson.M{"$set": bson.M{"status": store.RequestDeclined}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&req)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, refuse(ErrNotFound, "That request is no longer waiting for an answer.")
	}
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// MoveContact re-files one of the caller's contacts.
//
// Rule 3: this checks the destination sub-cap and not the budget. Only the
// caller's own filing changes - how the other person files the caller is theirs
// to decide, and moving does not touch their seats at all.
func (s *Service) MoveContact(ctx context.Context, caller bson.ObjectID, contactHex string, to capacity.Tier) (*store.Contact, error) {
	contactID, err := objectID(contactHex, "contact")
	if err != nil {
		return nil, err
	}

	var moved store.Contact
	err = s.Store.WithTransaction(ctx, func(ctx context.Context) error {
		if _, err := s.Store.LockSeats(ctx, caller); err != nil {
			return err
		}

		err := s.Store.Contacts.FindOne(ctx, bson.M{"_id": contactID, "ownerId": caller}).Decode(&moved)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return refuse(ErrNotFound, "That contact isn't on your list.")
		}
		if err != nil {
			return err
		}

		counts, err := s.Store.CountsFor(ctx, caller)
		if err != nil {
			return err
		}
		if err := capacity.CanMove(s.Caps, counts, moved.Tier, to); err != nil {
			return refuseCapacity(self(), to, s.Caps, counts, err)
		}

		if moved.Tier == to {
			return nil
		}
		if _, err := s.Store.Contacts.UpdateByID(ctx, contactID, bson.M{"$set": bson.M{"tier": to}}); err != nil {
			return err
		}
		moved.Tier = to
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &moved, nil
}

// RemoveContact deletes both sides of the pair, so the seat comes back to both
// people. Deleting one side would leave the other paying for a contact they can
// no longer see.
func (s *Service) RemoveContact(ctx context.Context, caller bson.ObjectID, contactHex string) (bool, error) {
	contactID, err := objectID(contactHex, "contact")
	if err != nil {
		return false, err
	}

	err = s.Store.WithTransaction(ctx, func(ctx context.Context) error {
		var contact store.Contact
		err := s.Store.Contacts.FindOne(ctx, bson.M{"_id": contactID, "ownerId": caller}).Decode(&contact)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return refuse(ErrNotFound, "That contact isn't on your list.")
		}
		if err != nil {
			return err
		}

		// Both seat counts change, so both users are locked - the same rule as
		// accept, which is what keeps a removal and an accept from interleaving.
		if _, err := s.Store.LockSeats(ctx, caller, contact.OtherID); err != nil &&
			!errors.Is(err, store.ErrUserNotFound) {
			return err
		}

		_, err = s.Store.Contacts.DeleteMany(ctx, bson.M{"$or": []bson.M{
			{"ownerId": caller, "otherId": contact.OtherID},
			{"ownerId": contact.OtherID, "otherId": caller},
		}})
		return err
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// Snapshot is what the People screen needs: the counts and the caps they are
// measured against, in one place so the client never derives a limit itself.
type Snapshot struct {
	Counts capacity.Counts
	Caps   capacity.Caps
}

func (s *Service) Snapshot(ctx context.Context, caller bson.ObjectID) (*Snapshot, error) {
	counts, err := s.Store.CountsFor(ctx, caller)
	if err != nil {
		return nil, err
	}
	return &Snapshot{Counts: counts, Caps: s.Caps}, nil
}
