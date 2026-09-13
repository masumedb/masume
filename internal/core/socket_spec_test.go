package core_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
)

// writeSocketFile writes a file where a server would open its socket. The path is read with
// a stat, so the kind of the file does not matter, and a real socket would not fit the path
// limit of a temporary directory on macOS.
func writeSocketFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("cannot write %s: %v", path, err)
	}
}

func TestIsSocketHostReadsEveryFormOfASocket(t *testing.T) {
	for _, held := range []struct {
		host   string
		socket bool
	}{
		{"socket", true},
		{"/var/run/postgresql", true},
		{"/tmp/mysql.sock", true},
		{"~/run/postgresql", true},
		{"127.0.0.1", false},
		{"db.internal", false},
		{"", false},
	} {
		if core.IsSocketHost(held.host) != held.socket {
			t.Errorf("%q reads as a socket: %v", held.host, !held.socket)
		}
	}
}

// Only the PostgreSQL and MySQL protocols connect over a unix socket.
func TestTakesSocketNamesTheEnginesThatDial(t *testing.T) {
	for _, engine := range []core.Engine{
		core.EnginePostgres, core.EngineTimescale, core.EngineMysql, core.EngineMariadb,
	} {
		if !core.TakesSocket(engine) {
			t.Errorf("%s takes no socket", engine)
		}
	}
	for _, engine := range []core.Engine{
		core.EngineSqlserver, core.EngineClickhouse, core.EngineMongo, core.EngineSqlite,
	} {
		if core.TakesSocket(engine) {
			t.Errorf("%s takes a socket", engine)
		}
	}
}

// A PostgreSQL client dials the directory, so the socket file the profile names is read
// back as its directory.
func TestFindSocketPathReadsThePostgresDirectory(t *testing.T) {
	directory := t.TempDir()
	writeSocketFile(t, filepath.Join(directory, ".s.PGSQL.5432"))

	for _, host := range []string{directory, filepath.Join(directory, ".s.PGSQL.5432")} {
		path, err := core.FindSocketPath(core.EnginePostgres, host, 5432)
		if err != nil {
			t.Fatalf("%s answered %v", host, err)
		}
		if path != directory {
			t.Errorf("%s reads as %q", host, path)
		}
	}
}

// A MySQL client dials the file, so a directory is read back as the socket inside it.
func TestFindSocketPathReadsTheMysqlFile(t *testing.T) {
	directory := t.TempDir()
	file := filepath.Join(directory, "mysqld.sock")
	writeSocketFile(t, file)

	for _, host := range []string{file, directory} {
		path, err := core.FindSocketPath(core.EngineMysql, host, 3306)
		if err != nil {
			t.Fatalf("%s answered %v", host, err)
		}
		if path != file {
			t.Errorf("%s reads as %q", host, path)
		}
	}
}

// A path with no socket names itself in the error.
func TestFindSocketPathReportsAPathThatIsNotThere(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")

	if _, err := core.FindSocketPath(core.EnginePostgres, missing, 5432); err == nil {
		t.Fatal("a missing path answered a socket")
	} else if !strings.Contains(err.Error(), missing) {
		t.Errorf("the error reads %q", err)
	}
}

// A directory without a MySQL socket names the files it was searched for.
func TestFindSocketPathReportsADirectoryWithoutASocket(t *testing.T) {
	directory := t.TempDir()

	if _, err := core.FindSocketPath(core.EngineMysql, directory, 3306); err == nil {
		t.Fatal("an empty directory answered a socket")
	} else if !strings.Contains(err.Error(), "mysqld.sock") {
		t.Errorf("the error reads %q", err)
	}
}

// The default paths of a machine without a local server name every path tried.
func TestFindSocketPathReportsTheDefaultsItTried(t *testing.T) {
	if _, err := os.Stat("/var/run/postgresql"); err == nil {
		t.Skip("this machine runs a local PostgreSQL socket directory")
	}

	_, err := core.FindSocketPath(core.EnginePostgres, core.SocketKeyword, 5432)
	if err == nil {
		t.Skip("this machine holds a default PostgreSQL socket")
	}
	for _, wanted := range []string{"/var/run/postgresql", "/tmp", ".s.PGSQL.5432"} {
		if !strings.Contains(err.Error(), wanted) {
			t.Errorf("the error %q names no %s", err, wanted)
		}
	}
}

// An engine that speaks neither protocol refuses a socket.
func TestFindSocketPathRefusesAnEngineWithoutSockets(t *testing.T) {
	if _, err := core.FindSocketPath(core.EngineMongo, "/tmp/mongo.sock", 27017); err == nil {
		t.Fatal("mongodb answered a socket path")
	}
}
