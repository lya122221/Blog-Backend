package repositories

import (
	"auth/internal/models"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

type userTestDriver struct{ conn *userTestConn }

func (d userTestDriver) Open(string) (driver.Conn, error) { return d.conn, nil }

type userTestConn struct {
	exec func(string, []driver.NamedValue) (driver.Result, error)
}

func (c *userTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}
func (c *userTestConn) Close() error { return nil }
func (c *userTestConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transaction not supported")
}
func (c *userTestConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return c.exec(query, args)
}

func TestAddNewUser(t *testing.T) {
	conn := &userTestConn{}
	sql.Register("auth-user-test", userTestDriver{conn: conn})
	db, err := sql.Open("auth-user-test", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	storage := &Storage{db: db}
	user := models.User{Username: "alice", Email: "alice@example.com", PasswordHash: "hash"}

	conn.exec = func(query string, args []driver.NamedValue) (driver.Result, error) {
		if !strings.Contains(query, "INSERT INTO users") || len(args) != 3 || args[0].Value != user.Username || args[1].Value != user.Email || args[2].Value != user.PasswordHash {
			t.Fatalf("unexpected insert: %q %v", query, args)
		}
		return driver.RowsAffected(1), nil
	}
	if err := storage.AddNewUser(user); err != nil {
		t.Fatalf("AddNewUser: %v", err)
	}

	conn.exec = func(string, []driver.NamedValue) (driver.Result, error) {
		return nil, &pgconn.PgError{Code: "23505"}
	}
	if err := storage.AddNewUser(user); !errors.Is(err, models.ErrUserExists) {
		t.Fatalf("duplicate user error: %v", err)
	}

	conn.exec = func(string, []driver.NamedValue) (driver.Result, error) {
		return nil, errors.New("database unavailable")
	}
	if err := storage.AddNewUser(user); err == nil || errors.Is(err, models.ErrUserExists) {
		t.Fatalf("database error: %v", err)
	}
}
