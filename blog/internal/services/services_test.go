package services

import (
	"blog/internal/models"
	"errors"
	"fmt"
	"testing"
	"time"
	"uuid"
)

type articlesRepoStub struct {
	articles                                 []models.Article
	article                                  *models.Article
	err                                      error
	limit, offset                            int
	tags                                     []string
	authorID, authorUsername, title, content string
	articleID                                uuid.UUID
	request                                  models.UpdateArticleRequest
	incremented                              chan uuid.UUID
}

func (r *articlesRepoStub) GetArticles(limit, offset int, tags []string) ([]models.Article, error) {
	r.limit, r.offset, r.tags = limit, offset, tags
	return r.articles, r.err
}
func (r *articlesRepoStub) CreateArticle(authorID, authorUsername, title, content string, tags []string) error {
	r.authorID, r.authorUsername, r.title, r.content, r.tags = authorID, authorUsername, title, content, tags
	return r.err
}
func (r *articlesRepoStub) GetArticleWithID(id uuid.UUID) (*models.Article, error) {
	r.articleID = id
	return r.article, r.err
}
func (r *articlesRepoStub) UpdateArticle(authorID string, id uuid.UUID, request models.UpdateArticleRequest) error {
	r.authorID, r.articleID, r.request = authorID, id, request
	return r.err
}
func (r *articlesRepoStub) DeleteArticle(authorID string, id uuid.UUID) error {
	r.authorID, r.articleID = authorID, id
	return r.err
}
func (r *articlesRepoStub) IncrementViewsCount(id uuid.UUID) error {
	if r.incremented != nil {
		r.incremented <- id
	}
	return r.err
}

func TestArticlesService(t *testing.T) {
	id := uuid.NewV4()
	idStr := fmt.Sprint(id)
	repo := &articlesRepoStub{articles: []models.Article{{ID: idStr}}, article: &models.Article{ID: idStr}, incremented: make(chan uuid.UUID, 1)}
	svc := NewArticlesService(repo)

	articles, err := svc.GetArticles(3, 10, []string{"go"})
	if err != nil || len(articles) != 1 || repo.limit != 10 || repo.offset != 20 {
		t.Fatalf("GetArticles result=%+v call=%d/%d err=%v", articles, repo.limit, repo.offset, err)
	}
	a := models.Article{Title: "title", Content: "body", Tags: []string{"go"}, Author: models.Author{ID: "author", Username: "alice"}}
	if err := svc.CreateArticle(a); err != nil || repo.authorID != "author" || repo.authorUsername != "alice" || repo.title != "title" {
		t.Fatalf("CreateArticle call not forwarded: %v %+v", err, repo)
	}
	got, err := svc.GetArticleWithID(idStr)
	if err != nil || got.ID != idStr {
		t.Fatalf("GetArticleWithID = %+v, %v", got, err)
	}
	select {
	case incremented := <-repo.incremented:
		if incremented != id {
			t.Fatalf("incremented %s", incremented)
		}
	case <-time.After(time.Second):
		t.Fatal("view increment was not started")
	}
	req := models.UpdateArticleRequest{Title: "new", Content: "new body", Tags: []string{"test"}}
	if err := svc.UpdateArticle("author", idStr, req); err != nil || repo.request.Title != "new" {
		t.Fatalf("UpdateArticle: %v", err)
	}
	if err := svc.DeleteArticle("author", idStr); err != nil || repo.articleID != id {
		t.Fatalf("DeleteArticle: %v", err)
	}
}

func TestArticlesServiceErrors(t *testing.T) {
	repoErr := errors.New("repo error")
	validID := "550e8400-e29b-41d4-a716-446655440000"
	svc := NewArticlesService(&articlesRepoStub{err: repoErr})
	if _, err := svc.GetArticles(1, 20, nil); !errors.Is(err, repoErr) {
		t.Fatalf("GetArticles error=%v", err)
	}
	if err := svc.CreateArticle(models.Article{}); !errors.Is(err, repoErr) {
		t.Fatalf("CreateArticle error=%v", err)
	}
	if _, err := svc.GetArticleWithID("bad"); err == nil {
		t.Fatal("expected UUID parse error")
	}
	if _, err := svc.GetArticleWithID(validID); !errors.Is(err, repoErr) {
		t.Fatalf("GetArticle error=%v", err)
	}
	if err := svc.UpdateArticle("a", "bad", models.UpdateArticleRequest{}); err == nil {
		t.Fatal("expected UUID parse error")
	}
	if err := svc.UpdateArticle("a", validID, models.UpdateArticleRequest{}); !errors.Is(err, repoErr) {
		t.Fatalf("Update error=%v", err)
	}
	if err := svc.DeleteArticle("a", "bad"); err == nil {
		t.Fatal("expected UUID parse error")
	}
	if err := svc.DeleteArticle("a", validID); !errors.Is(err, repoErr) {
		t.Fatalf("Delete error=%v", err)
	}
}

type interactionsRepoStub struct {
	comments                  []models.Comment
	liked                     bool
	count                     int
	err                       error
	articleID                 uuid.UUID
	userID, username, content string
}

func (r *interactionsRepoStub) GetComments(id uuid.UUID) ([]models.Comment, error) {
	r.articleID = id
	return r.comments, r.err
}
func (r *interactionsRepoStub) CreateComment(id uuid.UUID, userID, username, content string) error {
	r.articleID, r.userID, r.username, r.content = id, userID, username, content
	return r.err
}
func (r *interactionsRepoStub) ToggleLike(id uuid.UUID, userID string) (bool, int, error) {
	r.articleID, r.userID = id, userID
	return r.liked, r.count, r.err
}

func TestInteractionsService(t *testing.T) {
	id := uuid.NewV4()
	idStr := fmt.Sprint(id)
	repo := &interactionsRepoStub{comments: []models.Comment{{ID: "c1"}}, liked: true, count: 2}
	svc := NewInteractionsService(repo)
	comments, err := svc.GetComments(idStr)
	if err != nil || len(comments) != 1 {
		t.Fatalf("GetComments=%+v,%v", comments, err)
	}
	if err := svc.CreateComment(idStr, "u1", "alice", "hello"); err != nil || repo.content != "hello" || repo.username != "alice" {
		t.Fatalf("CreateComment=%v", err)
	}
	liked, count, err := svc.ToggleLike(idStr, "u1")
	if err != nil || !liked || count != 2 {
		t.Fatalf("ToggleLike=%v,%d,%v", liked, count, err)
	}

	repo.err = errors.New("repo")
	if _, err := svc.GetComments("bad"); err == nil {
		t.Fatal("expected parse error")
	}
	if _, err := svc.GetComments(idStr); err == nil {
		t.Fatal("expected repository error")
	}
	if err := svc.CreateComment("bad", "", "", ""); err == nil {
		t.Fatal("expected parse error")
	}
	if err := svc.CreateComment(idStr, "", "", ""); err == nil {
		t.Fatal("expected repository error")
	}
	if _, _, err := svc.ToggleLike("bad", ""); err == nil {
		t.Fatal("expected parse error")
	}
	if _, _, err := svc.ToggleLike(idStr, ""); err == nil {
		t.Fatal("expected repository error")
	}
}
