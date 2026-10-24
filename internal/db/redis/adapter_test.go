package redis

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
)

func TestConnectDialsTheTunnel(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	dialled := make(chan struct{}, 1)
	go func() {
		for {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			select {
			case dialled <- struct{}{}:
			default:
			}
			_ = connection.Close()
		}
	}()

	address := listener.Addr().(*net.TCPAddr)
	profile := cfg.Profile{
		Name: "tunnelled", Engine: core.EngineRedis, Host: "tunnel-only.invalid", Port: 6379,
		DialHost: "127.0.0.1", DialPort: address.Port,
	}
	ctx, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	if session, connectErr := NewAdapter(Support).Connect(ctx, profile, ""); connectErr == nil {
		_ = session.Close()
	}

	select {
	case <-dialled:
	default:
		t.Error("the connection did not go through the tunnel")
	}
}
