package detect_test

import (
	"encoding/base64"
	"strconv"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/detect"
)

const podList = `{"items": [
  {"metadata": {"name": "shop-db-0", "namespace": "shop"},
   "status": {"phase": "Running"},
   "spec": {"containers": [{
     "name": "postgres", "image": "docker.io/library/postgres:18",
     "ports": [{"containerPort": 5432, "protocol": "TCP"}],
     "env": [
       {"name": "POSTGRES_DB", "value": "orders"},
       {"name": "POSTGRES_USER", "valueFrom": {"configMapKeyRef": {"name": "db-settings", "key": "user"}}},
       {"name": "POSTGRES_PASSWORD", "valueFrom": {"secretKeyRef": {"name": "db-secret", "key": "password"}}}
     ]}]}},
  {"metadata": {"name": "cache", "namespace": "shop"},
   "status": {"phase": "Running"},
   "spec": {"containers": [{"name": "redis", "image": "redis:8-alpine",
     "env": [{"name": "REDIS_PASSWORD", "value": "plain"}]}]}},
  {"metadata": {"name": "web", "namespace": "shop"},
   "status": {"phase": "Running"},
   "spec": {"containers": [{"name": "app", "image": "nginx:1"}]}},
  {"metadata": {"name": "old-db", "namespace": "shop"},
   "status": {"phase": "Succeeded"},
   "spec": {"containers": [{"name": "postgres", "image": "postgres:18"}]}}
]}`

// fakeKubectl answers the Secret and the ConfigMap the pod list reads.
func fakeKubectl(arguments ...string) ([]byte, error) {
	joined := strings.Join(arguments, " ")
	switch {
	case strings.HasPrefix(joined, "get secret db-secret"):
		return []byte(`{"data": {"password": "` + base64.StdEncoding.EncodeToString([]byte("s3cret")) + `"}}`), nil
	case strings.HasPrefix(joined, "get configmap db-settings"):
		return []byte(`{"data": {"user": "shop_app"}}`), nil
	}
	return nil, nil
}

func TestBuildProfilesFromPodsReadsTheDatabasePods(t *testing.T) {
	found, err := detect.BuildProfilesFromPods([]byte(podList), "kind-dev", fakeKubectl)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("found %d profiles: %+v", len(found), found)
	}
	postgres, redis := found[0], found[1]
	if postgres.Name != "shop-db-0" || postgres.Engine != core.EnginePostgres ||
		postgres.Host != "127.0.0.1" || postgres.WaitForPort != postgres.Port {
		t.Errorf("the postgres profile is %+v", postgres)
	}
	wantedCommand := "kubectl port-forward --context kind-dev --namespace shop pod/shop-db-0 " +
		strconv.Itoa(postgres.Port) + ":5432"
	if postgres.Command != wantedCommand {
		t.Errorf("the command is %q, wanted %q", postgres.Command, wantedCommand)
	}
	if postgres.User != "shop_app" || postgres.Database != "orders" {
		t.Errorf("the user and database are %q and %q", postgres.User, postgres.Database)
	}
	if postgres.Auth != cfg.AuthCommand || postgres.Password != "" ||
		!strings.Contains(postgres.PasswordCommand, "kubectl get secret db-secret --namespace shop --context kind-dev") ||
		!strings.Contains(postgres.PasswordCommand, `index .data "password" | base64decode`) {
		t.Errorf("the password source is %s with %q", postgres.Auth, postgres.PasswordCommand)
	}
	if redis.Engine != core.EngineRedis || redis.Password != "plain" || redis.Auth != cfg.AuthPassword ||
		!strings.HasSuffix(redis.Command, ":6379") {
		t.Errorf("the redis profile is %+v", redis)
	}
}
