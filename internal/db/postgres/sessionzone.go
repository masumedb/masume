package postgres

import (
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/masumedb/masume/internal/db"
)

// sessionZones holds the location of each session TimeZone already loaded.
var sessionZones sync.Map

// findSessionZone returns the location of the session TimeZone. A name Go cannot load
// returns UTC.
func findSessionZone(connection *pgx.Conn) *time.Location {
	name := connection.PgConn().ParameterStatus("TimeZone")
	if held, found := sessionZones.Load(name); found {
		return held.(*time.Location)
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		location = time.UTC
	}
	sessionZones.Store(name, location)
	return location
}

// placeInSessionZone sets every moment of a zoned column to the session TimeZone. pgx
// returns a timestamptz in the local zone of the client.
func placeInSessionZone(connection *pgx.Conn, columns []db.ResultColumn, rows [][]any) {
	var location *time.Location
	for at, column := range columns {
		if !column.Zoned {
			continue
		}
		if location == nil {
			location = findSessionZone(connection)
		}
		for _, row := range rows {
			if at >= len(row) {
				continue
			}
			if held, isTime := row[at].(time.Time); isTime {
				row[at] = held.In(location)
			}
		}
	}
}
