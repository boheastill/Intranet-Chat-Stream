package bus

import "sync"

// broadcaster is the package-level SSE fan-out hub used by handlers.
var broadcaster = &Broadcaster{}

// SSEClient is a single connected SSE listener, optionally scoped to a channel.
type SSEClient struct {
	Channel string
	Message chan string
}

// Broadcaster fans out events to all registered SSE clients.
type Broadcaster struct {
	clients sync.Map
}

func (b *Broadcaster) Register(client *SSEClient) {
	b.clients.Store(client, true)
}

func (b *Broadcaster) Unregister(client *SSEClient) {
	b.clients.Delete(client)
	close(client.Message)
}

func (b *Broadcaster) Broadcast(channel string, eventPayload string) {
	b.clients.Range(func(key, value any) bool {
		client := key.(*SSEClient)
		if client.Channel == "" || client.Channel == channel {
			select {
			case client.Message <- eventPayload:
			default:
				// Drop if channel is full to prevent blocking
			}
		}
		return true
	})
}
