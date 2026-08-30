package graph_test

// The resolver layer, exercised over real HTTP.
//
// The service tests prove the rules. These prove the rules survive the trip
// through gqlgen and the error presenter - that a refusal arrives as a sentence
// with a stable code rather than a 500, that the caller header is enforced, and
// that a fault does not leak its internals. "The resolvers are thin" was an
// assertion made by reading until this file existed.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/tktaofik/capacity-takehome/api/graph"
	"github.com/tktaofik/capacity-takehome/api/internal/capacity"
	"github.com/tktaofik/capacity-takehome/api/internal/contacts"
	"github.com/tktaofik/capacity-takehome/api/internal/store"
)

// testCaps: a budget of 2 with a Pink cap of 1, small enough that every rule
// can be reached in a couple of calls.
func testCaps() capacity.Caps {
	return capacity.Caps{
		Budget: 2,
		PerTier: map[capacity.Tier]int{
			capacity.Pink:  1,
			capacity.Blue:  3,
			capacity.Green: 5,
		},
	}
}

type reply struct {
	Data   json.RawMessage
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	}
}

// code is the stable identifier the client branches on, or "" when the call
// succeeded.
func (r reply) code() string {
	if len(r.Errors) == 0 {
		return ""
	}
	if c, ok := r.Errors[0].Extensions["code"].(string); ok {
		return c
	}
	return "NO_CODE"
}

func (r reply) message() string {
	if len(r.Errors) == 0 {
		return ""
	}
	return r.Errors[0].Message
}

type harness struct {
	server *httptest.Server
	ids    map[string]bson.ObjectID
}

// newHarness wires the same server main.go does - schema, resolvers, and the
// real error presenter - in front of a real Mongo.
func newHarness(t *testing.T, caps capacity.Caps, names ...string) *harness {
	t.Helper()

	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27117/?replicaSet=rs0&directConnection=true"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	st, err := store.Connect(ctx, uri)
	if err != nil {
		t.Skipf("SKIPPED: no mongo at %s (%v). Run `make up`.", uri, err)
	}

	srv := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{
		Resolvers: &graph.Resolver{Store: st, Caps: caps, Service: contacts.New(st, caps)},
	}))
	srv.SetErrorPresenter(graph.PresentError)

	// The same header middleware the binary installs. Without it there is no
	// caller, and every authenticated field would fail for the wrong reason.
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if raw := r.Header.Get("X-User-Id"); raw != "" {
			if id, err := bson.ObjectIDFromHex(raw); err == nil {
				r = r.WithContext(store.WithUser(r.Context(), id))
			}
		}
		srv.ServeHTTP(w, r)
	})

	hs := &harness{server: httptest.NewServer(h), ids: map[string]bson.ObjectID{}}
	for _, n := range names {
		unique := fmt.Sprintf("%s-%d", n, time.Now().UnixNano())
		res, err := st.Users.InsertOne(context.Background(), store.User{Name: unique})
		if err != nil {
			t.Fatalf("insert %s: %v", n, err)
		}
		hs.ids[n] = res.InsertedID.(bson.ObjectID)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		all := make([]bson.ObjectID, 0, len(hs.ids))
		for _, id := range hs.ids {
			all = append(all, id)
		}
		in := bson.M{"$in": all}
		_, _ = st.Contacts.DeleteMany(ctx, bson.M{"$or": []bson.M{{"ownerId": in}, {"otherId": in}}})
		_, _ = st.Requests.DeleteMany(ctx, bson.M{"$or": []bson.M{{"fromId": in}, {"toId": in}}})
		_, _ = st.Users.DeleteMany(ctx, bson.M{"_id": in})
		hs.server.Close()
		_ = st.Client.Disconnect(ctx)
	})
	return hs
}

// post sends a query as `who`, or with no caller header when who is "".
func (h *harness) post(t *testing.T, who string, query string, vars map[string]any) reply {
	t.Helper()
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, h.server.URL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if who != "" {
		req.Header.Set("X-User-Id", h.ids[who].Hex())
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer res.Body.Close()

	var out reply
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

// connect drives send + accept through the API, failing the test if either is
// refused - it is setup, not the thing under test.
func (h *harness) connect(t *testing.T, from, to string, tier capacity.Tier) {
	t.Helper()
	sent := h.post(t, from, `mutation($t:ID!,$tier:Tier!){ sendRequest(toUserId:$t,tier:$tier){ id } }`,
		map[string]any{"t": h.ids[to].Hex(), "tier": string(tier)})
	if sent.code() != "" {
		t.Fatalf("setup send %s->%s: %s", from, to, sent.message())
	}
	var payload struct {
		SendRequest struct{ ID string } `json:"sendRequest"`
	}
	if err := json.Unmarshal(sent.Data, &payload); err != nil {
		t.Fatalf("setup decode: %v", err)
	}
	got := h.post(t, to, `mutation($r:ID!){ acceptRequest(requestId:$r){ id } }`,
		map[string]any{"r": payload.SendRequest.ID})
	if got.code() != "" {
		t.Fatalf("setup accept %s->%s: %s", from, to, got.message())
	}
}

// A refusal must reach the client as a readable sentence with a stable code,
// not as a 500 and not as a bare sentinel string.
func TestRefusalReachesTheClientAsASentence(t *testing.T) {
	h := newHarness(t, testCaps(), "me", "a", "b", "c")
	h.connect(t, "a", "me", capacity.Green)
	h.connect(t, "b", "me", capacity.Green) // me is now at 2 of 2

	sent := h.post(t, "c", `mutation($t:ID!){ sendRequest(toUserId:$t,tier:PINK){ id } }`,
		map[string]any{"t": h.ids["me"].Hex()})
	var payload struct {
		SendRequest struct{ ID string } `json:"sendRequest"`
	}
	if err := json.Unmarshal(sent.Data, &payload); err != nil {
		t.Fatalf("send: %v (%s)", err, sent.message())
	}

	// Pink is empty, so a refusal here can only be the budget. Rule 1, over HTTP.
	got := h.post(t, "me", `mutation($r:ID!){ acceptRequest(requestId:$r){ id } }`,
		map[string]any{"r": payload.SendRequest.ID})

	if got.code() != "BUDGET_FULL" {
		t.Fatalf("code: got %q, want BUDGET_FULL (message %q)", got.code(), got.message())
	}
	want := "You're using 2 of your 2 contact seats. Remove someone before you add anyone new."
	if got.message() != want {
		t.Fatalf("message:\n got %q\nwant %q", got.message(), want)
	}
	t.Logf("client was told: %q", got.message())
}

// The refusal about somebody else must not publish their numbers or tiers.
// Anyone able to send a request could otherwise probe a stranger's list.
func TestRefusalAboutSomeoneElseLeaksNothing(t *testing.T) {
	h := newHarness(t, testCaps(), "me", "sender", "x", "y")

	// The sender fills their own budget, then asks me - so the accept fails on
	// their side while mine is completely empty.
	h.connect(t, "x", "sender", capacity.Green)
	h.connect(t, "y", "sender", capacity.Green)

	sent := h.post(t, "sender", `mutation($t:ID!){ sendRequest(toUserId:$t,tier:GREEN){ id } }`,
		map[string]any{"t": h.ids["me"].Hex()})
	if sent.code() != "BUDGET_FULL" {
		t.Fatalf("a full sender should not be able to send: %q", sent.message())
	}

	// Reach past SendRequest's own budget check to get a pending request from a
	// sender who cannot honour it: the accept is where both sides are checked.
	h2 := newHarness(t, testCaps(), "me2", "sender2", "x2", "y2")
	pending := h2.post(t, "sender2", `mutation($t:ID!){ sendRequest(toUserId:$t,tier:GREEN){ id } }`,
		map[string]any{"t": h2.ids["me2"].Hex()})
	var payload struct {
		SendRequest struct{ ID string } `json:"sendRequest"`
	}
	if err := json.Unmarshal(pending.Data, &payload); err != nil {
		t.Fatalf("send: %v (%s)", err, pending.message())
	}
	h2.connect(t, "x2", "sender2", capacity.Green)
	h2.connect(t, "y2", "sender2", capacity.Green) // sender2 is now full

	got := h2.post(t, "me2", `mutation($r:ID!){ acceptRequest(requestId:$r){ id } }`,
		map[string]any{"r": payload.SendRequest.ID})

	if got.code() != "BUDGET_FULL" {
		t.Fatalf("code: got %q, want BUDGET_FULL (%q)", got.code(), got.message())
	}
	msg := got.message()
	t.Logf("accepter was told: %q", msg)

	// It must say who, and that it cannot go through - and nothing else.
	for _, leak := range []string{"2 of 2", "0 of", "1 of", "Pink", "Blue", "Green", "tier"} {
		if bytes.Contains([]byte(msg), []byte(leak)) {
			t.Errorf("refusal about another person leaks %q: %q", leak, msg)
		}
	}
	if !bytes.Contains([]byte(msg), []byte("can't take another contact")) {
		t.Errorf("refusal should say the person can't take another contact: %q", msg)
	}
}

// Rule 3 over HTTP: re-filing at a full budget is allowed, and the destination
// sub-cap is still enforced.
func TestMoveOverFullBudgetThroughTheAPI(t *testing.T) {
	h := newHarness(t, testCaps(), "me", "a", "b")
	h.connect(t, "a", "me", capacity.Green)
	h.connect(t, "b", "me", capacity.Green) // 2 of 2, Pink empty

	list := h.post(t, "me", `{ contacts { id tier } }`, nil)
	var contactsPayload struct {
		Contacts []struct{ ID, Tier string } `json:"contacts"`
	}
	if err := json.Unmarshal(list.Data, &contactsPayload); err != nil {
		t.Fatalf("contacts: %v (%s)", err, list.message())
	}
	if len(contactsPayload.Contacts) != 2 {
		t.Fatalf("expected 2 contacts, got %d", len(contactsPayload.Contacts))
	}

	// Allowed at a full budget: the seat is already spent.
	first := h.post(t, "me", `mutation($c:ID!){ moveContact(contactId:$c, tier:PINK){ tier } }`,
		map[string]any{"c": contactsPayload.Contacts[0].ID})
	if first.code() != "" {
		t.Fatalf("move at a full budget was refused: %s", first.message())
	}

	// The destination sub-cap still bites: Pink holds one.
	second := h.post(t, "me", `mutation($c:ID!){ moveContact(contactId:$c, tier:PINK){ tier } }`,
		map[string]any{"c": contactsPayload.Contacts[1].ID})
	if second.code() != "TIER_FULL" {
		t.Fatalf("second move: got %q, want TIER_FULL (%q)", second.code(), second.message())
	}
	t.Logf("client was told: %q", second.message())
}

// Without a caller header every authenticated field fails the same readable way.
func TestMissingCallerIsRefusedNotCrashed(t *testing.T) {
	h := newHarness(t, testCaps(), "me")

	for _, q := range []string{
		`{ me { id } }`,
		`{ contacts { id } }`,
		`{ capacity { budgetUsed } }`,
		`{ incomingRequests { id } }`,
		`mutation{ removeContact(contactId:"000000000000000000000000") }`,
	} {
		got := h.post(t, "", q, nil)
		if got.code() != "NO_CALLER" {
			t.Errorf("%s: got code %q (%q), want NO_CALLER", q, got.code(), got.message())
		}
	}
}

// Bad input is a refusal, not a 500, and never echoes an internal error.
func TestMalformedInputIsRefusedCleanly(t *testing.T) {
	h := newHarness(t, testCaps(), "me")

	got := h.post(t, "me", `mutation{ moveContact(contactId:"not-an-id", tier:PINK){ tier } }`, nil)
	if got.code() != "INVALID" {
		t.Fatalf("bad id: got %q (%q), want INVALID", got.code(), got.message())
	}
	if got.message() != "That contact id isn't valid." {
		t.Fatalf("bad id message: %q", got.message())
	}

	// A tier outside the enum never reaches a resolver; GraphQL rejects it, and
	// the message is about the query rather than about our internals.
	bad := h.post(t, "me", `mutation{ moveContact(contactId:"000000000000000000000000", tier:PURPLE){ tier } }`, nil)
	if len(bad.Errors) == 0 {
		t.Fatal("an unknown tier should not be accepted")
	}
	if bytes.Contains([]byte(bad.message()), []byte("mongo")) {
		t.Fatalf("validation error leaked internals: %q", bad.message())
	}
	t.Logf("unknown tier: %q", bad.message())
}

// The capacity query reports every tier, including empty ones, and never
// invents a limit the server does not hold.
func TestCapacityReportsEveryTier(t *testing.T) {
	h := newHarness(t, testCaps(), "me", "a")
	h.connect(t, "a", "me", capacity.Green)

	got := h.post(t, "me", `{ capacity { budgetUsed budgetCap tiers { tier used cap } } }`, nil)
	if got.code() != "" {
		t.Fatalf("capacity: %s", got.message())
	}
	var payload struct {
		Capacity struct {
			BudgetUsed, BudgetCap int
			Tiers                 []struct {
				Tier      string
				Used, Cap int
			}
		} `json:"capacity"`
	}
	if err := json.Unmarshal(got.Data, &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if payload.Capacity.BudgetCap != testCaps().Budget {
		t.Errorf("budgetCap: got %d, want %d", payload.Capacity.BudgetCap, testCaps().Budget)
	}
	if payload.Capacity.BudgetUsed != 1 {
		t.Errorf("budgetUsed: got %d, want 1", payload.Capacity.BudgetUsed)
	}
	if len(payload.Capacity.Tiers) != len(capacity.Tiers()) {
		t.Fatalf("tiers: got %d, want %d", len(payload.Capacity.Tiers), len(capacity.Tiers()))
	}
	for _, tier := range payload.Capacity.Tiers {
		want, _ := testCaps().Cap(capacity.Tier(tier.Tier))
		if tier.Cap != want {
			t.Errorf("%s cap: got %d, want %d", tier.Tier, tier.Cap, want)
		}
	}
}
