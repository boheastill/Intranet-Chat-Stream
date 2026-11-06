package bus

import (
	"sync"
	"testing"
)

// An SSE client disconnecting (tab closed, phone sleeps, wifi drops) must never
// take the server down. Previously Unregister closed the client channel while
// Broadcast was still doing `client.Message <- payload`, which panics with
// "send on closed channel" and kills the whole process.
func TestUnregisterDuringBroadcastDoesNotPanic(t *testing.T) {
	b := &Broadcaster{}
	var wg sync.WaitGroup

	for round := 0; round < 200; round++ {
		client := &SSEClient{Channel: "", Message: make(chan string, 10)}
		b.Register(client)

		wg.Add(2)
		go func() { defer wg.Done(); b.Broadcast("", "data: x\n\n") }()
		go func() { defer wg.Done(); b.Unregister(client) }()
		wg.Wait()

		// Drain so the buffered channel does not mask a leak.
		select {
		case <-client.Message:
		default:
		}
	}
}

// Broadcast must keep working for the clients that are still connected.
func TestBroadcastReachesConnectedClients(t *testing.T) {
	b := &Broadcaster{}
	live := &SSEClient{Channel: "", Message: make(chan string, 4)}
	b.Register(live)

	gone := &SSEClient{Channel: "", Message: make(chan string, 4)}
	b.Register(gone)
	b.Unregister(gone)

	b.Broadcast("", "data: hello\n\n")

	select {
	case msg := <-live.Message:
		if msg != "data: hello\n\n" {
			t.Fatalf("unexpected payload %q", msg)
		}
	default:
		t.Fatal("connected client did not receive the broadcast")
	}
}
