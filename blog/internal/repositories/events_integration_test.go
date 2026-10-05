package repositories

import (
	"blog/internal/models"
	"database/sql"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"uuid"
)

func TestArticleEventsPostgres(t *testing.T) {
	dsn := os.Getenv("BLOG_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set BLOG_TEST_POSTGRES_DSN to run the PostgreSQL integration test")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	schema := "article_events_test_" + strings.ReplaceAll(uuid.NewV4().String(), "-", "")
	if _, err := db.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	}()
	if _, err := db.Exec("SET search_path TO " + schema); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE articles (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			author_id UUID NOT NULL,
			author_username TEXT NOT NULL,
			title TEXT NOT NULL,
			content TEXT NOT NULL
		)`,
		`CREATE TABLE tags (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), name TEXT NOT NULL UNIQUE)`,
		`CREATE TABLE article_tags (
			article_id UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
			tag_id UUID NOT NULL REFERENCES tags(id),
			PRIMARY KEY (article_id, tag_id)
		)`,
		`CREATE TABLE comments (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			article_id UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
			user_id UUID NOT NULL,
			author_username TEXT NOT NULL,
			content TEXT NOT NULL
		)`,
		`CREATE TABLE likes (
			article_id UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
			user_id UUID NOT NULL,
			PRIMARY KEY (article_id, user_id)
		)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("create test table: %v", err)
		}
	}
	migration, err := os.ReadFile("../../migrations/000010_create_outbox_events_table.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(migration), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("apply outbox migration: %v", err)
		}
	}
	orderMigration, err := os.ReadFile("../../migrations/000011_order_outbox_events.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(orderMigration), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("apply ordered outbox migration: %v", err)
		}
	}
	for _, statement := range []string{
		`CREATE FUNCTION reject_outbox_insert() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			RAISE EXCEPTION 'outbox insert rejected by test';
		END;
		$$`,
		`CREATE TRIGGER reject_outbox_insert BEFORE INSERT ON outbox_events
		FOR EACH ROW EXECUTE FUNCTION reject_outbox_insert()`,
		`ALTER TABLE outbox_events DISABLE TRIGGER reject_outbox_insert`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("create outbox failure trigger: %v", err)
		}
	}

	s := &Storage{db: db}
	authorID := uuid.NewV4().String()
	userID := uuid.NewV4().String()
	reject := func(run func() error) {
		t.Helper()
		if _, err := db.Exec("ALTER TABLE outbox_events ENABLE TRIGGER reject_outbox_insert"); err != nil {
			t.Fatal(err)
		}
		actionErr := run()
		if _, err := db.Exec("ALTER TABLE outbox_events DISABLE TRIGGER reject_outbox_insert"); err != nil {
			t.Fatal(err)
		}
		if actionErr == nil {
			t.Fatal("action succeeded despite outbox insert failure")
		}
	}
	count := func(table string) int {
		t.Helper()
		var n int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	checkEvent := func(eventType string, articleID uuid.UUID, wantUserID, wantTitle string, wantTags []string) {
		t.Helper()
		var id, storedArticleID string
		var payload []byte
		if err := db.QueryRow(`
			SELECT event_id, article_id, payload
			FROM outbox_events WHERE event_type = $1
		`, eventType).Scan(&id, &storedArticleID, &payload); err != nil {
			t.Fatal(err)
		}
		var event models.AnalyticsEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			t.Fatal(err)
		}
		if _, err := uuid.Parse(event.ID); err != nil {
			t.Fatalf("invalid event ID: %v", err)
		}
		if id != event.ID || storedArticleID != articleID.String() || event.ArticleID != articleID.String() || event.Version != models.AnalyticsSchemaVersion || event.Type != eventType || event.AuthorID != authorID || event.UserID != wantUserID || event.Title != wantTitle || !slices.Equal(event.Tags, wantTags) || event.OccurredAt.IsZero() {
			t.Fatalf("invalid %s event: %+v", eventType, event)
		}
	}

	reject(func() error { return s.CreateArticle(authorID, "alice", "rejected", "body", nil) })
	if count("articles") != 0 || count("outbox_events") != 0 {
		t.Fatal("failed article creation was persisted")
	}
	if err := s.CreateArticle(authorID, "alice", "original", "body", []string{"go"}); err != nil {
		t.Fatal(err)
	}
	var articleID uuid.UUID
	if err := db.QueryRow("SELECT id FROM articles").Scan(&articleID); err != nil {
		t.Fatal(err)
	}
	checkEvent(models.ArticleCreated, articleID, "", "original", []string{"go"})

	reject(func() error {
		return s.UpdateArticle(authorID, articleID, models.UpdateArticleRequest{Title: "rejected", Content: "body", Tags: []string{"new"}})
	})
	var title string
	if err := db.QueryRow("SELECT title FROM articles WHERE id = $1", articleID).Scan(&title); err != nil || title != "original" || count("article_tags") != 1 {
		t.Fatalf("failed article update was persisted: title=%s, err=%v", title, err)
	}
	if err := s.UpdateArticle(authorID, articleID, models.UpdateArticleRequest{Title: "updated", Content: "body", Tags: []string{"new"}}); err != nil {
		t.Fatal(err)
	}
	checkEvent(models.ArticleUpdated, articleID, "", "updated", []string{"new"})

	reject(func() error { return s.CreateComment(articleID, userID, "bob", "rejected") })
	if count("comments") != 0 {
		t.Fatal("failed comment was persisted")
	}
	if err := s.CreateComment(articleID, userID, "bob", "hello"); err != nil {
		t.Fatal(err)
	}
	checkEvent(models.CommentCreated, articleID, userID, "", nil)

	reject(func() error { _, _, err := s.ToggleLike(articleID, userID); return err })
	if count("likes") != 0 {
		t.Fatal("failed like was persisted")
	}
	if liked, likesCount, err := s.ToggleLike(articleID, userID); err != nil || !liked || likesCount != 1 {
		t.Fatalf("like: liked=%v, count=%d, err=%v", liked, likesCount, err)
	}
	checkEvent(models.ArticleLiked, articleID, userID, "", nil)
	reject(func() error { _, _, err := s.ToggleLike(articleID, userID); return err })
	if count("likes") != 1 {
		t.Fatal("failed unlike was persisted")
	}
	if liked, likesCount, err := s.ToggleLike(articleID, userID); err != nil || liked || likesCount != 0 {
		t.Fatalf("unlike: liked=%v, count=%d, err=%v", liked, likesCount, err)
	}
	checkEvent(models.ArticleUnliked, articleID, userID, "", nil)

	reject(func() error { return s.DeleteArticle(authorID, articleID) })
	if count("articles") != 1 || count("outbox_events") != 5 {
		t.Fatal("failed article deletion was persisted")
	}
	if err := s.DeleteArticle(authorID, articleID); err != nil {
		t.Fatal(err)
	}
	if count("articles") != 0 || count("outbox_events") != 6 {
		t.Fatal("article deletion or outbox event is missing")
	}
	checkEvent(models.ArticleDeleted, articleID, "", "", nil)
}
