package cassandra

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/gocql/gocql"

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
		Name: "tunnelled", Engine: core.EngineCassandra, Host: "tunnel-only.invalid", Port: 9042,
		DialHost: "127.0.0.1", DialPort: address.Port,
	}
	if session, connectErr := NewAdapter(Support).Connect(context.Background(), profile, ""); connectErr == nil {
		_ = session.Close()
	}

	select {
	case <-dialled:
	case <-time.After(time.Second):
		t.Error("the connection did not go through the tunnel")
	}
}

func TestRouteThroughTunnelMapsEveryNodeToTheTunnel(t *testing.T) {
	cluster := gocql.NewCluster("127.0.0.1:40123")
	routeThroughTunnel(cluster, "127.0.0.1", 40123)

	address, port := cluster.AddressTranslator.Translate(net.ParseIP("10.0.0.7"), 9042)
	if !address.Equal(net.ParseIP("127.0.0.1")) || port != 40123 {
		t.Errorf("a node address maps to %s:%d, wanted the tunnel", address, port)
	}
}
