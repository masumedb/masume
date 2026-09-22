package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// A profile writes this word in place of a host to use the default socket of the engine.
const SocketKeyword = "socket"

// The prefix of the socket file a PostgreSQL server listens on, before the port.
const postgresSocketPrefix = ".s.PGSQL."

// The default socket paths, in resolve order. postgres takes the socket directory, mysql
// the socket file.
var (
	postgresSocketDirectories = []string{"/var/run/postgresql", "/run/postgresql", "/tmp"}
	mysqlSocketFiles          = []string{
		"/var/run/mysqld/mysqld.sock", "/run/mysqld/mysqld.sock",
		"/tmp/mysql.sock", "/var/lib/mysql/mysql.sock",
	}
)

// mysqlSocketNames are the file names a MySQL socket takes inside a directory.
var mysqlSocketNames = []string{"mysqld.sock", "mysql.sock"}

// IsSocketHost is true for a host that names a unix socket: an absolute path, a path under
// the home directory, or the socket keyword.
func IsSocketHost(host string) bool {
	return host == SocketKeyword ||
		strings.HasPrefix(host, "/") || strings.HasPrefix(host, "~/")
}

// TakesSocket is true for an engine that connects over a unix socket.
func TakesSocket(engine Engine) bool {
	family := ResolveEngineInfo(engine).Family
	return family == FamilyPostgres || family == FamilyMysql
}

// FindSocketPath returns the socket the driver dials: the path the host names, or the first
// default path of the engine that exists. postgres resolves to the socket directory, mysql
// to the socket file.
func FindSocketPath(engine Engine, host string, port int) (string, error) {
	if runtime.GOOS == "windows" {
		return "", fmt.Errorf(
			"windows has no unix socket for %s; write a host and a port", engine)
	}
	if !TakesSocket(engine) {
		return "", fmt.Errorf("%s does not connect over a unix socket", engine)
	}
	if host == SocketKeyword {
		return findDefaultSocket(engine, port)
	}

	path := ExpandHomePath(host)
	held, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("the socket %s is unreadable: %w", path, err)
	}
	if ResolveEngineInfo(engine).Family == FamilyPostgres {
		return readPostgresSocketDirectory(path, held.IsDir()), nil
	}
	if !held.IsDir() {
		return path, nil
	}
	return findMysqlSocketFile(path)
}

// readPostgresSocketDirectory returns the directory the client dials. A profile that names
// the socket file itself keeps its directory.
func readPostgresSocketDirectory(path string, isDirectory bool) string {
	if isDirectory {
		return path
	}
	if strings.HasPrefix(filepath.Base(path), postgresSocketPrefix) {
		return filepath.Dir(path)
	}
	return path
}

// findMysqlSocketFile returns the socket file inside that directory.
func findMysqlSocketFile(directory string) (string, error) {
	for _, name := range mysqlSocketNames {
		path := filepath.Join(directory, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("%s holds no %s", directory, strings.Join(mysqlSocketNames, " or "))
}

// findDefaultSocket returns the first default socket of the engine that exists.
func findDefaultSocket(engine Engine, port int) (string, error) {
	if ResolveEngineInfo(engine).Family == FamilyPostgres {
		for _, directory := range postgresSocketDirectories {
			if _, err := os.Stat(filepath.Join(
				directory, postgresSocketPrefix+strconv.Itoa(port))); err == nil {
				return directory, nil
			}
		}
		return "", fmt.Errorf(
			"no %s%d socket in %s; write the path in host",
			postgresSocketPrefix, port, strings.Join(postgresSocketDirectories, ", "))
	}

	for _, path := range mysqlSocketFiles {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no socket at %s; write the path in host",
		strings.Join(mysqlSocketFiles, ", "))
}
