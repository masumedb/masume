package mongo

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
)

func buildProbeProfile(user string) cfg.Profile {
	return cfg.Profile{
		Name: "orders", Engine: core.EngineMongo, Host: "cluster.example.com",
		Port: 27017, Database: "shop", User: user,
	}
}

// The driver reports that it chose no server and writes the whole topology after it. The
// reason sits inside that, and the reason is what the user has to act on.
func TestDescribeConnectFailureAnswersTheReasonAndNotTheTopology(t *testing.T) {
	err := errors.New("server selection error: context deadline exceeded, " +
		"current topology: { Type: Unknown, Servers: [{ Addr: cluster.example.com:27017, " +
		"Type: Unknown, Last error: tls: failed to verify certificate: x509: " +
		"certificate signed by unknown authority }, ] }")

	written := DescribeConnectFailure(buildProbeProfile("reader"), err)
	if !strings.HasSuffix(written, "x509: certificate signed by unknown authority") {
		t.Errorf("the message reads %q", written)
	}
	if strings.Contains(written, "topology") {
		t.Errorf("the message carries the whole topology:\n%s", written)
	}
	if !strings.Contains(written, "cluster.example.com") {
		t.Errorf("the message does not name the server:\n%s", written)
	}
}

// An error that carries no topology is written as it stands.
func TestDescribeConnectFailureKeepsAnErrorItCannotShorten(t *testing.T) {
	written := DescribeConnectFailure(buildProbeProfile("reader"), errors.New("no route to host"))
	if !strings.Contains(written, "no route to host") {
		t.Errorf("the message reads %q", written)
	}
}

// A server that authenticates every command refuses one that is not authenticated. Where
// the profile names no user, that is the reason, and the server does not say it.
func TestBuildAuthenticationMessageNamesTheMissingUser(t *testing.T) {
	err := errors.New("(Unauthorized) Command buildInfo requires authentication")

	written := BuildAuthenticationMessage(buildProbeProfile(""), err)
	if !strings.Contains(written, "authentication requires a user; the profile user is missing") {
		t.Errorf("the message reads %q", written)
	}
	// A profile that does name a user is told what the server said.
	written = BuildAuthenticationMessage(buildProbeProfile("reader"), err)
	if !strings.Contains(written, "requires authentication") {
		t.Errorf("the message reads %q", written)
	}
}

// Only the codes that say who the connection is count as an authentication failure. Any
// other refusal leaves the connection open, because the server answered it.
func TestIsAuthenticationErrorNamesOnlyTheCodesOfWhoTheConnectionIs(t *testing.T) {
	for _, held := range []struct {
		code int32
		want bool
	}{
		{13, true},  // Unauthorized
		{18, true},  // AuthenticationFailed
		{26, false}, // NamespaceNotFound
		{59, false}, // CommandNotFound
	} {
		err := mongo.CommandError{Code: held.code, Message: "reported"}
		if answered := IsAuthenticationError(err); answered != held.want {
			t.Errorf("code %d reads as %v, wanted %v", held.code, answered, held.want)
		}
	}
	if IsAuthenticationError(errors.New("no route to host")) {
		t.Error("an error the server never sent reads as one about who the connection is")
	}
}

func TestBuildClientOptionsConnectsDirectly(t *testing.T) {
	profile := buildProbeProfile("reader")
	profile.DirectConnection = true
	built, err := BuildClientOptions(profile, "")
	if err != nil {
		t.Fatalf("the options do not build: %v", err)
	}
	if built.Direct == nil || !*built.Direct {
		t.Error("the options do not connect directly")
	}
}

func TestBuildClientOptionsSetsTheAuthSourceAndTheReplicaSet(t *testing.T) {
	profile := buildProbeProfile("reader")
	profile.AuthSource, profile.ReplicaSet = "users", "rs0"
	built, err := BuildClientOptions(profile, "secret")
	if err != nil {
		t.Fatalf("the options do not build: %v", err)
	}
	if built.Auth == nil || built.Auth.AuthSource != "users" {
		t.Errorf("the credential reads %+v, wanted the auth source users", built.Auth)
	}
	if built.ReplicaSet == nil || *built.ReplicaSet != "rs0" {
		t.Errorf("the replica set reads %v, wanted rs0", built.ReplicaSet)
	}
}

func TestBuildClientOptionsReadsTheHostsOfAnSRVRecord(t *testing.T) {
	profile := buildProbeProfile("reader")
	profile.Host, profile.SRV, profile.SSLMode = "cluster.example.invalid", true, core.SSLVerifyFull
	built, err := BuildClientOptions(profile, "secret")
	if err != nil {
		t.Fatalf("the options do not build: %v", err)
	}
	if slices.Contains(built.Hosts, "cluster.example.invalid:27017") {
		t.Errorf("the options dial the record name: %v", built.Hosts)
	}
	if built.TLSConfig == nil || built.TLSConfig.ServerName != "" {
		t.Errorf("the TLS config is %+v, wanted one without a server name", built.TLSConfig)
	}
}

func TestBuildClientOptionsRefusesAnSRVRecordThroughATunnel(t *testing.T) {
	profile := buildProbeProfile("reader")
	profile.SRV, profile.SSHHost = true, "bastion"
	if _, err := BuildClientOptions(profile, ""); err == nil {
		t.Error("an SRV record through a tunnel builds options")
	}
	profile.SSHHost, profile.DirectConnection = "", true
	if _, err := BuildClientOptions(profile, ""); err == nil {
		t.Error("an SRV record with a direct connection builds options")
	}
}

func TestBuildClientOptionsTakesTheOptionsOfTheProfile(t *testing.T) {
	profile := buildProbeProfile("reader")
	profile.Options = map[string]string{
		"maxPoolSize": "3", "appName": "etl", "retryWrites": "false", "authMechanism": "SCRAM-SHA-256",
	}
	held, err := BuildClientOptions(profile, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := held.Validate(); err != nil {
		t.Fatal(err)
	}
	if *held.MaxPoolSize != 3 || *held.AppName != "etl" || *held.RetryWrites ||
		held.Auth.AuthMechanism != "SCRAM-SHA-256" || held.Auth.Password != "secret" {
		t.Errorf("the options are pool %d, app %q, retry %v, mechanism %q",
			*held.MaxPoolSize, *held.AppName, *held.RetryWrites, held.Auth.AuthMechanism)
	}
	if !slices.Equal(held.Hosts, []string{"cluster.example.com:27017"}) {
		t.Errorf("the hosts are %v", held.Hosts)
	}
}

func TestBuildClientOptionsReportsAnOptionValueTheDriverCannotRead(t *testing.T) {
	profile := buildProbeProfile("")
	profile.Options = map[string]string{"maxPoolSize": "many"}
	held, err := BuildClientOptions(profile, "")
	if err == nil {
		err = held.Validate()
	}
	if err == nil || !strings.Contains(err.Error(), "maxPoolSize") {
		t.Errorf("the error is %v", err)
	}
}

func TestBuildClientOptionsSeedsEveryHost(t *testing.T) {
	profile := buildProbeProfile("")
	profile.OtherHosts = []string{"db2.example.com:27018"}
	held, err := BuildClientOptions(profile, "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(held.Hosts, []string{"cluster.example.com:27017", "db2.example.com:27018"}) {
		t.Errorf("the hosts are %v", held.Hosts)
	}
}
