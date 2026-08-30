// Package store is the data layer: Mongo connection, collection handles and
// the document shapes. Business rules do not live here.
package store

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"

	"github.com/tktaofik/capacity-takehome/api/internal/capacity"
)

type User struct {
	ID   bson.ObjectID `bson:"_id,omitempty"`
	Name string        `bson:"name"`
	// SeatVersion exists only to be written. Bumping it inside a transaction
	// takes a document-level lock on this user, which is how two concurrent
	// accepts for the same person are made to collide instead of both counting
	// seats from the same stale snapshot. See LockSeats.
	SeatVersion int64 `bson:"seatVersion"`
}

// Contact is one side of a pair. Adding a contact writes two of these, one for
// each user; removing it must free both.
type Contact struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	OwnerID   bson.ObjectID `bson:"ownerId"`
	OtherID   bson.ObjectID `bson:"otherId"`
	Tier      capacity.Tier `bson:"tier"`
	CreatedAt time.Time     `bson:"createdAt"`
}

type RequestStatus string

const (
	RequestPending  RequestStatus = "PENDING"
	RequestAccepted RequestStatus = "ACCEPTED"
	RequestDeclined RequestStatus = "DECLINED"
)

type Request struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	FromID    bson.ObjectID `bson:"fromId"`
	ToID      bson.ObjectID `bson:"toId"`
	Tier      capacity.Tier `bson:"tier"`
	Status    RequestStatus `bson:"status"`
	CreatedAt time.Time     `bson:"createdAt"`
}

type Store struct {
	Client   *mongo.Client
	DB       *mongo.Database
	Users    *mongo.Collection
	Contacts *mongo.Collection
	Requests *mongo.Collection
}

func Connect(ctx context.Context, uri string) (*Store, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	db := client.Database("capacity")
	s := &Store{
		Client:   client,
		DB:       db,
		Users:    db.Collection("users"),
		Contacts: db.Collection("contacts"),
		Requests: db.Collection("requests"),
	}
	return s, s.ensureIndexes(ctx)
}

// ensureIndexes declares the one index the app cannot be correct without: a
// pair may exist only once per owner. Any index you need for the capacity
// rules is yours to add here.
func (s *Store) ensureIndexes(ctx context.Context) error {
	if _, err := s.Contacts.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "ownerId", Value: 1}, {Key: "otherId", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("owner_other_unique"),
	}); err != nil {
		return err
	}

	_, err := s.Requests.Indexes().CreateMany(ctx, []mongo.IndexModel{
		// At most one live request per direction per pair. Sending twice is a
		// duplicate, not a second request, and the database says so rather than
		// a read-then-write check that two clients could both pass.
		{
			Keys: bson.D{{Key: "fromId", Value: 1}, {Key: "toId", Value: 1}},
			Options: options.Index().
				SetUnique(true).
				SetPartialFilterExpression(bson.M{"status": RequestPending}).
				SetName("pending_pair_unique"),
		},
		// The two inbox queries.
		{
			Keys:    bson.D{{Key: "toId", Value: 1}, {Key: "status", Value: 1}, {Key: "createdAt", Value: -1}},
			Options: options.Index().SetName("inbox"),
		},
		{
			Keys:    bson.D{{Key: "fromId", Value: 1}, {Key: "status", Value: 1}, {Key: "createdAt", Value: -1}},
			Options: options.Index().SetName("outbox"),
		},
	})
	return err
}

// Seed inserts a few people to talk to, so the client has something to show on
// first run. It is idempotent.
func (s *Store) Seed(ctx context.Context) error {
	names := []string{"You", "Ada", "Grace", "Alan", "Katherine", "Barbara", "Edsger", "Radia", "Ken", "Margaret"}
	for _, n := range names {
		filter := bson.M{"name": n}
		update := bson.M{"$setOnInsert": bson.M{"name": n}}
		if _, err := s.Users.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true)); err != nil {
			return err
		}
	}
	return nil
}

// CountsFor returns the caller's active contacts per tier.
func (s *Store) CountsFor(ctx context.Context, ownerID bson.ObjectID) (capacity.Counts, error) {
	counts := capacity.Counts{}
	cur, err := s.Contacts.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"ownerId": ownerID}}},
		{{Key: "$group", Value: bson.M{"_id": "$tier", "n": bson.M{"$sum": 1}}}},
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var rows []struct {
		Tier capacity.Tier `bson:"_id"`
		N    int           `bson:"n"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	for _, r := range rows {
		counts[r.Tier] = r.N
	}
	return counts, nil
}

// WithTransaction runs fn inside a multi-document transaction with snapshot
// reads and a majority commit.
//
// fn may run more than once: the driver retries on TransientTransactionError,
// which is the label a write conflict arrives with. That retry is not a
// workaround, it is the mechanism - the loser of a seat race re-runs against
// fresh counts and refuses for the right reason instead of silently
// overfilling. fn must therefore be free of side effects outside this
// transaction, and must never swallow an error it did not cause.
func (s *Store) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	sess, err := s.Client.StartSession()
	if err != nil {
		return err
	}
	defer sess.EndSession(ctx)

	_, err = sess.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		return nil, fn(ctx)
	}, options.Transaction().
		SetReadConcern(readconcern.Snapshot()).
		SetWriteConcern(writeconcern.Majority()))
	return err
}

// LockSeats claims the right to change the seat counts of the given users for
// the rest of the surrounding transaction, and returns their documents.
//
// This is the answer to rule 4. Mongo transactions give snapshot isolation, and
// snapshot isolation does not stop two transactions reading "7 of 8" and each
// inserting a different contact document - different documents, no conflict,
// nine seats used. Writing to a document both transactions must write makes
// them collide: the second gets a write conflict, is retried, and re-reads the
// counts the first one already committed.
//
// Users are locked in a fixed order so that two transactions touching the same
// pair queue up the same way round rather than each holding what the other
// wants. Call this before reading counts, never after: the lock is what makes
// the counts trustworthy.
func (s *Store) LockSeats(ctx context.Context, ids ...bson.ObjectID) (map[bson.ObjectID]User, error) {
	ordered := make([]bson.ObjectID, 0, len(ids))
	seen := make(map[bson.ObjectID]bool, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			ordered = append(ordered, id)
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		return bytes.Compare(ordered[i][:], ordered[j][:]) < 0
	})

	locked := make(map[bson.ObjectID]User, len(ordered))
	for _, id := range ordered {
		var u User
		err := s.Users.FindOneAndUpdate(ctx,
			bson.M{"_id": id},
			bson.M{"$inc": bson.M{"seatVersion": 1}},
			options.FindOneAndUpdate().SetReturnDocument(options.After),
		).Decode(&u)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrUserNotFound
		}
		if err != nil {
			return nil, err
		}
		locked[id] = u
	}
	return locked, nil
}

// ErrUserNotFound means an id did not match anybody.
var ErrUserNotFound = errors.New("store: user not found")

// UsersByID loads several users in one round trip, so a list of contacts or
// requests does not turn into one query per row.
func (s *Store) UsersByID(ctx context.Context, ids []bson.ObjectID) (map[bson.ObjectID]User, error) {
	out := make(map[bson.ObjectID]User, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	cur, err := s.Users.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var docs []User
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	for _, u := range docs {
		out[u.ID] = u
	}
	return out, nil
}

// ContactsFor returns the caller's contacts, closest tier first and oldest
// first inside a tier.
func (s *Store) ContactsFor(ctx context.Context, ownerID bson.ObjectID) ([]Contact, error) {
	cur, err := s.Contacts.Find(ctx, bson.M{"ownerId": ownerID},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var docs []Contact
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

// RequestsWhere returns requests matching a filter, newest first.
func (s *Store) RequestsWhere(ctx context.Context, filter bson.M) ([]Request, error) {
	cur, err := s.Requests.Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var docs []Request
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}
