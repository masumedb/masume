package cassandra

import (
	"strings"
	"testing"
	"time"

	"github.com/gocql/gocql"
)

func TestApplyClusterOptionsWritesTheSettings(t *testing.T) {
	cluster := gocql.NewCluster("db:9042")
	err := applyClusterOptions(cluster, map[string]string{
		"timeout": "7s", "consistency": "local_quorum", "proto_version": "4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cluster.Timeout != 7*time.Second || cluster.Consistency != gocql.LocalQuorum || cluster.ProtoVersion != 4 {
		t.Errorf("timeout %v, consistency %v, protocol %d", cluster.Timeout, cluster.Consistency, cluster.ProtoVersion)
	}
}

func TestApplyClusterOptionsRefusesWhatItCannotRead(t *testing.T) {
	for options, wanted := range map[string]string{
		"nonsense": "unknown option nonsense", "timeout": "option timeout",
	} {
		err := applyClusterOptions(gocql.NewCluster("db"), map[string]string{options: "x"})
		if err == nil || !strings.Contains(err.Error(), wanted) {
			t.Errorf("%s answered %v", options, err)
		}
	}
}
