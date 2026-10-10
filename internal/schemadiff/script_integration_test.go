//go:build integration

package schemadiff_test

import (
	"context"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/db/engines"
)

const dropScriptSchemas = "drop schema if exists masume_s1 cascade; drop schema if exists masume_s2 cascade"

func TestScriptMakesThePostgresSourceEqualTheTarget(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, dropScriptSchemas,
		"create schema masume_s1", "create schema masume_s2",
		`create table masume_s1.orders (id serial primary key,
			total integer not null check (total > 0), note text, legacy int)`,
		"create index orders_total on masume_s1.orders (total)",
		"create table masume_s1.archive (id int)",
		"create view masume_s1.old_orders as select id from masume_s1.orders",
		`create table masume_s2.customers (id serial primary key, name text not null unique)`,
		`create table masume_s2.orders (id serial primary key,
			total bigint not null default 0 check (total > 0), note text not null,
			placed_at timestamptz, customer_id int references masume_s2.customers (id))`,
		"create index orders_placed on masume_s2.orders (placed_at)",
		"create view masume_s2.big_orders as select id, total from masume_s2.orders where total > 100")
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(), dropScriptSchemas, dbtest.ReadEverything, nil)
	})

	if left := migrate(t, session, session, "masume_s1", "masume_s2"); len(left) != 0 {
		t.Errorf("after the script the schemas still differ: %+v", left)
	}
	shown, err := session.RunQuery(context.Background(),
		"select current_schema()", dbtest.ReadEverything, nil)
	if err != nil || len(shown.Rows) != 1 || shown.Rows[0][0] != "public" {
		t.Errorf("after the script the session schema is %v: %v", shown.Rows, err)
	}
}

func openMysqlDatabase(t *testing.T, database string) db.Session {
	t.Helper()
	profile, password := dbtest.BuildProfile(t, dbtest.MySQL)
	profile.Database = database
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestScriptMakesTheMysqlSourceEqualTheTarget(t *testing.T) {
	setup := dbtest.Open(t, dbtest.MySQL)
	dbtest.RunStatements(t, setup,
		"drop database if exists masume_s1", "drop database if exists masume_s2",
		"create database masume_s1", "create database masume_s2",
		`create table masume_s1.orders (id int auto_increment primary key,
			total int not null, note varchar(20), legacy int, check (total > 0))`,
		"create index orders_total on masume_s1.orders (total)",
		"create table masume_s1.archive (id int)",
		`create table masume_s2.customers (id int auto_increment primary key,
			name varchar(50) not null, constraint customers_name unique (name))`,
		`create table masume_s2.orders (id int auto_increment primary key,
			total bigint not null default 0, note varchar(20) not null default 'none',
			placed_at datetime, customer_id int,
			constraint orders_customer foreign key (customer_id) references masume_s2.customers (id))`,
		"create index orders_placed on masume_s2.orders (placed_at)",
		"create view masume_s2.big_orders as select id, total from masume_s2.orders where total > 100")
	t.Cleanup(func() {
		_, _ = setup.RunQuery(context.Background(), "drop database if exists masume_s1", dbtest.ReadEverything, nil)
		_, _ = setup.RunQuery(context.Background(), "drop database if exists masume_s2", dbtest.ReadEverything, nil)
	})

	source := openMysqlDatabase(t, "masume_s1")
	target := openMysqlDatabase(t, "masume_s2")
	if left := migrate(t, source, target, "masume_s1", "masume_s2"); len(left) != 0 {
		t.Errorf("after the script the schemas still differ: %+v", left)
	}
}

func openSqlserverDatabase(t *testing.T, database string) db.Session {
	t.Helper()
	profile, password := dbtest.BuildProfile(t, dbtest.Sqlserver)
	profile.Database = database
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestScriptMakesTheSqlserverSourceEqualTheTarget(t *testing.T) {
	setup := dbtest.Open(t, dbtest.Sqlserver)
	dropDatabases := func() {
		for _, database := range []string{"masume_s1", "masume_s2"} {
			_, _ = setup.RunQuery(context.Background(), "if db_id('"+database+"') is not null begin "+
				"alter database "+database+" set single_user with rollback immediate "+
				"drop database "+database+" end", dbtest.ReadEverything, nil)
		}
	}
	dropDatabases()
	t.Cleanup(dropDatabases)
	dbtest.RunStatements(t, setup, "create database masume_s1", "create database masume_s2")

	source := openSqlserverDatabase(t, "masume_s1")
	dbtest.RunStatements(t, source,
		`create table orders (id int identity(1,1) primary key,
			total int not null default 5 constraint orders_positive check (total > 0),
			note varchar(20), legacy int default 1)`,
		"create index orders_total on orders (total)",
		"create table archive (id int)",
		"create view old_orders as select id from orders")
	target := openSqlserverDatabase(t, "masume_s2")
	dbtest.RunStatements(t, target,
		`create table customers (id int identity(1,1) primary key,
			name nvarchar(50) not null, constraint customers_name unique (name))`,
		`create table orders (id int identity(1,1) primary key,
			total bigint not null default 0 constraint orders_positive check (total > 0),
			note varchar(20) not null default 'none', placed_at datetime2,
			customer_id int constraint orders_customer references customers (id))`,
		"create index orders_placed on orders (placed_at)",
		"create view big_orders as select id, total from orders where total > 100")

	if left := migrate(t, source, target, "dbo", "dbo"); len(left) != 0 {
		t.Errorf("after the script the schemas still differ: %+v", left)
	}
}
