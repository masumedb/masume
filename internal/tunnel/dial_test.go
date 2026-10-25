package tunnel

import (
	"errors"
	"net"
	"testing"
	"time"
)

// A dial that the SSH library never answers ends with the connection of the client.
func TestDialThroughEndsWithTheSession(t *testing.T) {
	never := make(chan struct{})
	defer close(never)
	dial := func(string, string) (net.Conn, error) {
		<-never
		return nil, errors.New("closed")
	}
	ended := make(chan struct{})
	close(ended)

	answered := make(chan error, 1)
	go func() {
		_, err := dialThrough(dial, ended, "127.0.0.1:1")
		answered <- err
	}()
	select {
	case err := <-answered:
		if !errors.Is(err, errSessionEnded) {
			t.Errorf("the dial answered %v, wanted the end of the session", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the dial still waits after the session ended")
	}
}
