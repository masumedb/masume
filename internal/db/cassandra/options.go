package cassandra

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gocql/gocql"
)

// clusterOptions are the option names the cluster settings take.
var clusterOptions = []string{
	"connect_timeout", "consistency", "disable_initial_host_lookup", "local_dc",
	"proto_version", "timeout",
}

// applyClusterOptions writes the options into the cluster settings.
func applyClusterOptions(cluster *gocql.ClusterConfig, options map[string]string) error {
	for _, name := range slices.Sorted(maps.Keys(options)) {
		value := options[name]
		var err error
		switch name {
		case "timeout":
			cluster.Timeout, err = time.ParseDuration(value)
		case "connect_timeout":
			cluster.ConnectTimeout, err = time.ParseDuration(value)
		case "consistency":
			err = cluster.Consistency.UnmarshalText([]byte(strings.ToUpper(value)))
		case "proto_version":
			cluster.ProtoVersion, err = strconv.Atoi(value)
		case "disable_initial_host_lookup":
			cluster.DisableInitialHostLookup, err = strconv.ParseBool(value)
		case "local_dc":
			cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(
				gocql.DCAwareRoundRobinPolicy(value))
		default:
			return fmt.Errorf("unknown option %s; use one of %s", name, strings.Join(clusterOptions, ", "))
		}
		if err != nil {
			return fmt.Errorf("option %s: %w", name, err)
		}
	}
	return nil
}
