package store_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/tktaofik/capacity-takehome/api/internal/capacity"
	"github.com/tktaofik/capacity-takehome/api/internal/contacts"
	"github.com/tktaofik/capacity-takehome/api/internal/store"
)

// testCaps for the race: a budget of exactly one seat, which is the smallest
// arrangement that still has a last seat to fight over. Green is roomy so the
// sub-cap never gets a chance to be the reason anything failed.
func testCaps(budget int) capacity.Caps {
	return capacity.Caps{
		Budget: budget,
		PerTier: map[capacity.Tier]int{
			capacity.Pink:  1,
			capacity.Blue:  3,
			capacity.Green: 50,
		},
	}
}

// dial connects to the same Mongo the app uses, in a database of its own so a
// test run never touches whatever is in the dev data.
func dial(t *testing.T) *store.Store {
	t.Helper()

	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27117/?replicaSet=rs0&directConnection=true"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	st, err := store.Connect(ctx, uri)
	if err != nil {
		t.Skipf("SKIPPED, and rule 4 is therefore unproven: no mongo at %s (%v).\n"+
			"Run `make up` and re-run this test - it is the only check that the last seat "+
			"cannot be sold twice.", uri, err)
	}
	return st
}

// fixture is two users and a clean slate, torn down afterwards.
type fixture struct {
	svc   *contacts.Service
	store *store.Store
	ids   []bson.ObjectID
}

func newFixture(t *testing.T, caps capacity.Caps, names ...string) *fixture {
	t.Helper()
	st := dial(t)
	ctx := context.Background()

	f := &fixture{svc: contacts.New(st, caps), store: st}
	for _, name := range names {
		res, err := st.Users.InsertOne(ctx, store.User{Name: fmt.Sprintf("%s-%d", name, time.Now().UnixNano())})
		if err != nil {
			t.Fatalf("insert user %s: %v", name, err)
		}
		f.ids = append(f.ids, res.InsertedID.(bson.ObjectID))
	}

	t.Cleanup(func() {
		ctx := context.Background()
		owners := bson.M{"$in": f.ids}
		_, _ = st.Contacts.DeleteMany(ctx, bson.M{"$or": []bson.M{{"ownerId": owners}, {"otherId": owners}}})
		_, _ = st.Requests.DeleteMany(ctx, bson.M{"$or": []bson.M{{"fromId": owners}, {"toId": owners}}})
		_, _ = st.Users.DeleteMany(ctx, bson.M{"_id": owners})
		_ = st.Client.Disconnect(ctx)
	})
	return f
}

// pendingRequest writes a PENDING request straight to the collection, skipping
// SendRequest so the setup cannot be refused by the very rules under test.
func (f *fixture) pendingRequest(t *testing.T, from, to bson.ObjectID, tier capacity.Tier) string {
	t.Helper()
	res, err := f.store.Requests.InsertOne(context.Background(), store.Request{
		FromID:    from,
		ToID:      to,
		Tier:      tier,
		Status:    store.RequestPending,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed request: %v", err)
	}
	return res.InsertedID.(bson.ObjectID).Hex()
}

// Rule 4 - two accepts landing at the same moment on a user with exactly one
// free seat must not both succeed. Exactly one wins; the other fails cleanly.
//
// This one needs a real Mongo (make up), which is why it lives here and not in
// the capacity package. Read-then-write will pass a serial test and fail this
// one - that is the point of it.
func TestConcurrentAcceptsTakeOneSeat(t *testing.T) {
	const budget = 1
	f := newFixture(t, testCaps(budget), "receiver", "sender-a", "sender-b")
	receiver, senderA, senderB := f.ids[0], f.ids[1], f.ids[2]

	// Two people have asked. The receiver has room for exactly one of them.
	requestA := f.pendingRequest(t, senderA, receiver, capacity.Green)
	requestB := f.pendingRequest(t, senderB, receiver, capacity.Green)

	// Both accepts are released at the same instant, which is what stops this
	// from quietly degrading into a serial test that always passes.
	var start sync.WaitGroup
	start.Add(1)
	var wg sync.WaitGroup
	errs := make([]error, 2)

	for i, requestID := range []string{requestA, requestB} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start.Wait()
			_, errs[i] = f.svc.AcceptRequest(context.Background(), receiver, requestID)
		}()
	}
	start.Done()
	wg.Wait()

	won, lost := 0, 0
	for i, err := range errs {
		switch {
		case err == nil:
			won++
		case errors.Is(err, capacity.ErrBudgetFull):
			lost++
		default:
			t.Fatalf("accept %d failed for the wrong reason: %v", i, err)
		}
	}
	if won != 1 || lost != 1 {
		t.Fatalf("both accepts raced one free seat: %d succeeded, %d refused for capacity (want 1 and 1); errors: %v", won, lost, errs)
	}

	// The refusal has to be readable, not just correct. A silent 500 would pass
	// the count above and still fail the brief.
	for _, err := range errs {
		if err == nil {
			continue
		}
		refusal, ok := contacts.IsRefusal(err)
		if !ok {
			t.Fatalf("the losing accept was not a user-facing refusal: %v", err)
		}
		if refusal.Message == "" {
			t.Fatal("the losing accept refused with an empty message")
		}
		t.Logf("loser was told: %q", refusal.Message)
	}

	// The database has to agree with the answers we gave. One seat spent, and
	// the winning pair written on both sides.
	assertSeats(t, f.store, receiver, 1)
	assertPairSymmetric(t, f.store, receiver)
}

// Both sides of an accept are checked, so a full sender fails the accept even
// when the accepter has all the room in the world. Rule 2, second half.
func TestAcceptChecksBothSides(t *testing.T) {
	f := newFixture(t, testCaps(1), "receiver", "sender", "bystander")
	receiver, sender, bystander := f.ids[0], f.ids[1], f.ids[2]

	// The sender spends their only seat on someone else first.
	spend := f.pendingRequest(t, bystander, sender, capacity.Green)
	if _, err := f.svc.AcceptRequest(context.Background(), sender, spend); err != nil {
		t.Fatalf("setup accept: %v", err)
	}

	// Now their earlier request to the receiver cannot be honoured, even though
	// the receiver is completely empty.
	stale := f.pendingRequest(t, sender, receiver, capacity.Green)
	_, err := f.svc.AcceptRequest(context.Background(), receiver, stale)
	if !errors.Is(err, capacity.ErrBudgetFull) {
		t.Fatalf("accepting from a full sender: got %v, want ErrBudgetFull", err)
	}

	refusal, ok := contacts.IsRefusal(err)
	if !ok {
		t.Fatalf("refusal was not user-facing: %v", err)
	}
	// The sentence has to say whose list is full, or the receiver will think it
	// is their own.
	t.Logf("receiver was told: %q", refusal.Message)
	if refusal.Message == "" {
		t.Fatal("empty refusal message")
	}

	assertSeats(t, f.store, receiver, 0)
}

// Removing a contact frees the seat on both sides, not just the remover's.
// Rule R4 of the build list, and the thing a one-sided delete gets wrong.
func TestRemoveFreesBothSeats(t *testing.T) {
	f := newFixture(t, testCaps(1), "alice", "bob")
	alice, bob := f.ids[0], f.ids[1]
	ctx := context.Background()

	req := f.pendingRequest(t, bob, alice, capacity.Green)
	contact, err := f.svc.AcceptRequest(ctx, alice, req)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	assertSeats(t, f.store, alice, 1)
	assertSeats(t, f.store, bob, 1)

	if _, err := f.svc.RemoveContact(ctx, alice, contact.ID.Hex()); err != nil {
		t.Fatalf("remove: %v", err)
	}
	assertSeats(t, f.store, alice, 0)
	assertSeats(t, f.store, bob, 0)
}

// Re-filing over a full budget works against a real database too, not just in
// the pure rules. Rule 3, end to end.
func TestMoveOverFullBudget(t *testing.T) {
	caps := capacity.Caps{
		Budget:  1,
		PerTier: map[capacity.Tier]int{capacity.Pink: 1, capacity.Blue: 1, capacity.Green: 1},
	}
	f := newFixture(t, caps, "alice", "bob")
	alice, bob := f.ids[0], f.ids[1]
	ctx := context.Background()

	req := f.pendingRequest(t, bob, alice, capacity.Green)
	contact, err := f.svc.AcceptRequest(ctx, alice, req)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}

	// Alice is at 1 of 1. An add would be refused; a move must not be.
	moved, err := f.svc.MoveContact(ctx, alice, contact.ID.Hex(), capacity.Pink)
	if err != nil {
		t.Fatalf("moving at a full budget: %v", err)
	}
	if moved.Tier != capacity.Pink {
		t.Fatalf("tier after move: got %s, want %s", moved.Tier, capacity.Pink)
	}

	// Bob's side is untouched: how he files Alice is his business.
	var bobSide store.Contact
	if err := f.store.Contacts.FindOne(ctx, bson.M{"ownerId": bob, "otherId": alice}).Decode(&bobSide); err != nil {
		t.Fatalf("bob's side: %v", err)
	}
	if bobSide.Tier != capacity.Green {
		t.Fatalf("bob's tier changed to %s; a move is one-sided", bobSide.Tier)
	}
}

// The pending-request index refuses a second live request to the same person,
// rather than relying on a read that two clients could both pass.
func TestDuplicatePendingRequestRefused(t *testing.T) {
	f := newFixture(t, testCaps(8), "alice", "bob")
	alice, bob := f.ids[0], f.ids[1]
	ctx := context.Background()

	if _, err := f.svc.SendRequest(ctx, alice, bob.Hex(), capacity.Green); err != nil {
		t.Fatalf("first send: %v", err)
	}
	_, err := f.svc.SendRequest(ctx, alice, bob.Hex(), capacity.Green)
	if !errors.Is(err, contacts.ErrConflict) {
		t.Fatalf("second send: got %v, want ErrConflict", err)
	}
}

// A pending request holds no seat, so one free seat carries many of them.
// Rule 2, against a real database this time.
func TestPendingRequestsHoldNoSeat(t *testing.T) {
	f := newFixture(t, testCaps(1), "alice", "b1", "b2", "b3", "b4")
	alice := f.ids[0]
	ctx := context.Background()

	for _, target := range f.ids[1:] {
		if _, err := f.svc.SendRequest(ctx, alice, target.Hex(), capacity.Green); err != nil {
			t.Fatalf("send to %s on a budget of 1: %v", target.Hex(), err)
		}
	}
	// Four requests out, still nothing spent.
	assertSeats(t, f.store, alice, 0)

	n, err := f.store.Requests.CountDocuments(ctx, bson.M{"fromId": alice, "status": store.RequestPending})
	if err != nil {
		t.Fatalf("count requests: %v", err)
	}
	if n != 4 {
		t.Fatalf("outstanding requests: got %d, want 4", n)
	}
}

func assertSeats(t *testing.T, st *store.Store, owner bson.ObjectID, want int) {
	t.Helper()
	counts, err := st.CountsFor(context.Background(), owner)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if got := counts.Total(); got != want {
		t.Fatalf("seats used by %s: got %d, want %d (%v)", owner.Hex(), got, want, counts)
	}
}

// assertPairSymmetric checks that every contact of owner has a matching row the
// other way round. A pair written on one side only would leave someone paying
// for a contact they cannot see.
func assertPairSymmetric(t *testing.T, st *store.Store, owner bson.ObjectID) {
	t.Helper()
	ctx := context.Background()
	rows, err := st.ContactsFor(ctx, owner)
	if err != nil {
		t.Fatalf("contacts: %v", err)
	}
	for _, c := range rows {
		err := st.Contacts.FindOne(ctx, bson.M{"ownerId": c.OtherID, "otherId": owner}).Err()
		if errors.Is(err, mongo.ErrNoDocuments) {
			t.Fatalf("contact %s exists on one side only", c.ID.Hex())
		}
		if err != nil {
			t.Fatalf("mirror lookup: %v", err)
		}
	}
}
