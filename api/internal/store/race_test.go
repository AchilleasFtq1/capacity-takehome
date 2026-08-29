package store_test

import "testing"

// Rule 4 - two accepts landing at the same moment on a user with exactly one
// free seat must not both succeed. Exactly one wins; the other fails cleanly.
//
// This one needs a real Mongo (make up), which is why it lives here and not in
// the capacity package. Read-then-write will pass a serial test and fail this
// one - that is the point of it.
func TestConcurrentAcceptsTakeOneSeat(t *testing.T) {
	t.Skip("delete this line and write the test")
}
