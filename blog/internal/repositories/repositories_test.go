package repositories

import (
	"blog/internal/models"
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestUUIDPgxEncoding(t *testing.T) {
	id := uuid.NewV4()
	encoded, err := pgtype.NewMap().Encode(pgtype.UUIDOID, pgtype.BinaryFormatCode, id, nil)
	if err != nil {
		t.Fatalf("encode UUID: %v", err)
	}
	if !bytes.Equal(encoded, id[:]) {
		t.Fatalf("encoded UUID does not match original: %x", encoded)
	}
}

type testDriver struct{ conn *testConn }

func (d testDriver) Open(string) (driver.Conn, error) { return d.conn, nil }

type testConn struct {
	query               func(string, []driver.NamedValue) (driver.Rows, error)
	exec                func(string, []driver.NamedValue) (driver.Result, error)
	beginErr, commitErr error
}

func (c *testConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}
func (c *testConn) Close() error                             { return nil }
func (c *testConn) CheckNamedValue(*driver.NamedValue) error { return nil }
func (c *testConn) Begin() (driver.Tx, error) {
	if c.beginErr != nil {
		return nil, c.beginErr
	}
	return &testTx{commitErr: c.commitErr}, nil
}
func (c *testConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) { return c.Begin() }
func (c *testConn) QueryContext(_ context.Context, q string, a []driver.NamedValue) (driver.Rows, error) {
	if c.query == nil {
		return nil, errors.New("unexpected query")
	}
	return c.query(q, a)
}
func (c *testConn) ExecContext(_ context.Context, q string, a []driver.NamedValue) (driver.Result, error) {
	if c.exec == nil {
		return nil, errors.New("unexpected exec")
	}
	return c.exec(q, a)
}

type testTx struct{ commitErr error }

func (t *testTx) Commit() error   { return t.commitErr }
func (t *testTx) Rollback() error { return nil }

type testRows struct {
	columns  []string
	values   [][]driver.Value
	index    int
	finalErr error
}

func (r *testRows) Columns() []string { return r.columns }
func (r *testRows) Close() error      { return nil }
func (r *testRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		if r.finalErr != nil {
			err := r.finalErr
			r.finalErr = nil
			return err
		}
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
func rows(columns []string, values ...[]driver.Value) driver.Rows {
	return &testRows{columns: columns, values: values}
}

var driverNumber atomic.Int64

func openTestDB(t *testing.T, c *testConn) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("blog-test-%d", driverNumber.Add(1))
	sql.Register(name, testDriver{conn: c})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
func has(q, fragment string) bool {
	return strings.Contains(strings.Join(strings.Fields(q), " "), fragment)
}

func articleValues(id string) []driver.Value {
	return []driver.Value{id, "title", "body", 7, time.Now(), "u1", "alice", "{go,test}"}
}

func TestArticleReadRepository(t *testing.T) {
	c := &testConn{}
	c.query = func(q string, a []driver.NamedValue) (driver.Rows, error) {
		if has(q, "users") {
			t.Fatalf("article read depends on blog users: %s", q)
		}
		switch {
		case has(q, "WHERE tags.name = ANY"):
			if len(a) != 3 {
				t.Fatalf("tagged args=%v", a)
			}
			return rows([]string{"id", "title", "content", "views", "created", "author_id", "username", "tags"}, articleValues("a2")), nil
		case has(q, "ORDER BY articles.created_at DESC"):
			if len(a) != 2 {
				t.Fatalf("plain args=%v", a)
			}
			return rows([]string{"id", "title", "content", "views", "created", "author_id", "username", "tags"}, articleValues("a1")), nil
		case has(q, "WHERE articles.id = $1"):
			v := articleValues("a1")
			v[5], v[6] = v[6], v[5]
			return rows([]string{"id", "title", "content", "views", "created", "username", "author_id", "tags"}, v), nil
		default:
			return nil, fmt.Errorf("unexpected query: %s", q)
		}
	}
	s := &Storage{db: openTestDB(t, c)}
	got, err := s.GetArticles(10, 20, nil)
	if err != nil || len(got) != 1 || got[0].ID != "a1" || len(got[0].Tags) != 2 {
		t.Fatalf("plain articles=%+v err=%v", got, err)
	}
	got, err = s.GetArticles(5, 0, []string{"go"})
	if err != nil || len(got) != 1 || got[0].ID != "a2" {
		t.Fatalf("tagged articles=%+v err=%v", got, err)
	}
	id := uuid.NewV4()
	article, err := s.GetArticleWithID(id)
	if err != nil || article.Author.Username != "alice" || article.Author.ID != "u1" {
		t.Fatalf("article=%+v err=%v", article, err)
	}

	c.query = func(string, []driver.NamedValue) (driver.Rows, error) { return nil, errors.New("query failed") }
	if _, err := s.GetArticles(1, 0, nil); err == nil {
		t.Fatal("expected list error")
	}
	if _, err := s.GetArticleWithID(id); err == nil {
		t.Fatal("expected article error")
	}
}

func TestArticleWriteRepository(t *testing.T) {
	c := &testConn{}
	c.query = func(q string, a []driver.NamedValue) (driver.Rows, error) {
		switch {
		case has(q, "INSERT INTO articles"):
			if len(a) != 4 || a[0].Value != "author-1" || a[1].Value != "alice" {
				t.Fatalf("unexpected article insert: %v", a)
			}
			return rows([]string{"id"}, []driver.Value{"article-1"}), nil
		case has(q, "INSERT INTO tags"):
			return rows([]string{"id"}, []driver.Value{"tag-1"}), nil
		case has(q, "SELECT author_id"):
			return rows([]string{"author_id"}, []driver.Value{"author-1"}), nil
		default:
			return nil, fmt.Errorf("unexpected query: %s", q)
		}
	}
	execCalls := 0
	var events []models.AnalyticsEvent
	c.exec = func(q string, a []driver.NamedValue) (driver.Result, error) {
		execCalls++
		if has(q, "INSERT INTO outbox_events") {
			var event models.AnalyticsEvent
			if err := json.Unmarshal([]byte(a[3].Value.(string)), &event); err != nil {
				return nil, err
			}
			if a[0].Value != event.ID || a[1].Value != event.Type || a[2].Value != event.ArticleID || event.Version != models.AnalyticsSchemaVersion || event.OccurredAt.IsZero() {
				return nil, fmt.Errorf("invalid outbox event: %+v", event)
			}
			events = append(events, event)
		}
		return driver.RowsAffected(1), nil
	}
	s := &Storage{db: openTestDB(t, c)}
	if err := s.CreateArticle("author-1", "alice", "title", "body", []string{"go", "test"}); err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}
	id := uuid.NewV4()
	request := models.UpdateArticleRequest{Title: "new", Content: "new body", Tags: []string{"go"}}
	if err := s.UpdateArticle("author-1", id, request); err != nil {
		t.Fatalf("UpdateArticle: %v", err)
	}
	if err := s.DeleteArticle("author-1", id); err != nil {
		t.Fatalf("DeleteArticle: %v", err)
	}
	if err := s.UpdateArticleViews(4, id); err != nil {
		t.Fatalf("UpdateArticleViews: %v", err)
	}
	if execCalls < 6 {
		t.Fatalf("too few writes: %d", execCalls)
	}
	if len(events) != 3 || events[0].Type != models.ArticleCreated || events[0].AuthorID != "author-1" || events[0].Title != "title" || len(events[0].Tags) != 2 || events[1].Type != models.ArticleUpdated || events[1].Title != "new" || events[2].Type != models.ArticleDeleted {
		t.Fatalf("article events = %+v", events)
	}

	if err := s.UpdateArticle("other", id, request); err == nil {
		t.Fatal("expected update authorization error")
	}
	if err := s.DeleteArticle("other", id); err == nil {
		t.Fatal("expected delete authorization error")
	}
	c.beginErr = errors.New("begin")
	if err := s.CreateArticle("a", "alice", "t", "c", nil); err == nil {
		t.Fatal("expected begin error")
	}
	if err := s.UpdateArticle("a", id, request); err == nil {
		t.Fatal("expected begin error")
	}
	if err := s.DeleteArticle("a", id); err == nil {
		t.Fatal("expected begin error")
	}
	c.beginErr = nil
	c.exec = func(string, []driver.NamedValue) (driver.Result, error) { return nil, errors.New("exec") }
	if err := s.UpdateArticleViews(1, id); err == nil {
		t.Fatal("expected views update error")
	}
}

func TestCreateArticleWithoutAuthorUsername(t *testing.T) {
	c := &testConn{}
	s := &Storage{db: openTestDB(t, c)}
	if err := s.CreateArticle("author-1", "", "title", "body", nil); err == nil {
		t.Fatal("expected missing username error")
	}
}

func TestInteractionsRepository(t *testing.T) {
	c := &testConn{}
	likedRows := int64(0)
	var events []models.AnalyticsEvent
	c.query = func(q string, a []driver.NamedValue) (driver.Rows, error) {
		if has(q, "users") {
			t.Fatalf("comment read depends on blog users: %s", q)
		}
		if has(q, "SELECT COUNT(*)") {
			return rows([]string{"count"}, []driver.Value{int64(3)}), nil
		}
		if has(q, "SELECT author_id") {
			return rows([]string{"author_id"}, []driver.Value{"article-author"}), nil
		}
		return rows([]string{"id", "article_id", "author_id", "username", "content", "created"}, []driver.Value{"c1", "a1", "u1", "alice", "hello", time.Now()}), nil
	}
	c.exec = func(q string, a []driver.NamedValue) (driver.Result, error) {
		if has(q, "INSERT INTO outbox_events") {
			var event models.AnalyticsEvent
			if err := json.Unmarshal([]byte(a[3].Value.(string)), &event); err != nil {
				return nil, err
			}
			if a[0].Value != event.ID || a[1].Value != event.Type || a[2].Value != event.ArticleID || event.Version != models.AnalyticsSchemaVersion || event.OccurredAt.IsZero() {
				return nil, fmt.Errorf("invalid outbox event: %+v", event)
			}
			events = append(events, event)
		}
		if has(q, "INSERT INTO comments") {
			if len(a) != 4 || a[1].Value != "u1" || a[2].Value != "alice" || a[3].Value != "hello" {
				t.Fatalf("unexpected comment insert args: %+v", a)
			}
		}
		if has(q, "DELETE FROM likes") {
			return driver.RowsAffected(likedRows), nil
		}
		return driver.RowsAffected(1), nil
	}
	s := &Storage{db: openTestDB(t, c)}
	id := uuid.NewV4()
	comments, err := s.GetComments(id)
	if err != nil || len(comments) != 1 || comments[0].Content != "hello" || comments[0].Author.ID != "u1" || comments[0].Author.Username != "alice" {
		t.Fatalf("comments=%+v err=%v", comments, err)
	}
	if err := s.CreateComment(id, "u1", "alice", "hello"); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	liked, count, err := s.ToggleLike(id, "u1")
	if err != nil || !liked || count != 3 {
		t.Fatalf("like=%v count=%d err=%v", liked, count, err)
	}
	likedRows = 1
	liked, count, err = s.ToggleLike(id, "u1")
	if err != nil || liked || count != 3 {
		t.Fatalf("unlike=%v count=%d err=%v", liked, count, err)
	}
	if len(events) != 3 || events[0].Type != models.CommentCreated || events[1].Type != models.ArticleLiked || events[2].Type != models.ArticleUnliked {
		t.Fatalf("interaction events = %+v", events)
	}
	for _, event := range events {
		if event.AuthorID != "article-author" || event.UserID != "u1" || event.ArticleID != id.String() {
			t.Fatalf("unexpected interaction event: %+v", event)
		}
	}

	c.query = func(string, []driver.NamedValue) (driver.Rows, error) { return nil, errors.New("query") }
	if _, err := s.GetComments(id); err == nil {
		t.Fatal("expected comments error")
	}
	c.exec = func(string, []driver.NamedValue) (driver.Result, error) { return nil, errors.New("exec") }
	if err := s.CreateComment(id, "u", "alice", "x"); err == nil {
		t.Fatal("expected comment error")
	}
	if _, _, err := s.ToggleLike(id, "u"); err == nil {
		t.Fatal("expected toggle error")
	}
}

func TestCreateCommentWithoutAuthorUsername(t *testing.T) {
	c := &testConn{}
	s := &Storage{db: openTestDB(t, c)}
	if err := s.CreateComment(uuid.NewV4(), "author-1", "", "hello"); err == nil {
		t.Fatal("expected missing username error")
	}
}
