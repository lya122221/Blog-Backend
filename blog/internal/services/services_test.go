package services

import (
	"blog/internal/models"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type userRepoStub struct {
	id, hash string
	err      error
	added    models.User
}

func (r *userRepoStub) GetUserData(string) (string, string, error) { return r.id, r.hash, r.err }
func (r *userRepoStub) AddNewUser(user models.User) error          { r.added = user; return r.err }

func TestUserServiceRegisterAndLogin(t *testing.T) {
	t.Setenv("JWTKEY", "test-secret")
	repo := &userRepoStub{}
	svc := NewUserService(repo)

	registration := &models.UserRegister{Email: "a@example.com", Username: "alice", Password: "secret"}
	if err := svc.Register(registration); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if repo.added.Email != registration.Email || repo.added.Username != registration.Username {
		t.Fatalf("unexpected user: %+v", repo.added)
	}
	if !ComparePasswords(repo.added.PasswordHash, registration.Password) {
		t.Fatal("stored password is not a bcrypt hash of the password")
	}
	if ComparePasswords(repo.added.PasswordHash, "wrong") {
		t.Fatal("wrong password accepted")
	}

	repo.id, repo.hash = "user-1", repo.added.PasswordHash
	tokenString, err := svc.Login(&models.UserLogin{Email: registration.Email, Password: registration.Password})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	token, err := jwt.Parse(tokenString, func(*jwt.Token) (any, error) { return []byte(os.Getenv("JWTKEY")), nil })
	if err != nil || !token.Valid {
		t.Fatalf("invalid generated token: %v", err)
	}
	if got := token.Claims.(jwt.MapClaims)["user_id"]; got != "user-1" {
		t.Fatalf("user_id = %v", got)
	}
}

func TestUserServiceErrors(t *testing.T) {
	hash, err := GenerateHashedPassword("secret")
	if err != nil {
		t.Fatal(err)
	}

	repoErr := errors.New("repository unavailable")
	if err := NewUserService(&userRepoStub{err: repoErr}).Register(&models.UserRegister{Password: "secret"}); !errors.Is(err, repoErr) {
		t.Fatalf("Register error = %v", err)
	}
	if _, err := NewUserService(&userRepoStub{err: repoErr}).Login(&models.UserLogin{}); !errors.Is(err, repoErr) {
		t.Fatalf("Login error = %v", err)
	}
	if _, err := NewUserService(&userRepoStub{id: "1", hash: hash}).Login(&models.UserLogin{Password: "wrong"}); err == nil {
		t.Fatal("expected incorrect-password error")
	}
}

type articlesRepoStub struct {
	articles                 []models.Article
	article                  *models.Article
	err                      error
	limit, offset            int
	tags                     []string
	authorID, title, content string
	articleID                uuid.UUID
	request                  models.UpdateArticleRequest
	incremented              chan uuid.UUID
}

func (r *articlesRepoStub) GetArticles(limit, offset int, tags []string) ([]models.Article, error) {
	r.limit, r.offset, r.tags = limit, offset, tags
	return r.articles, r.err
}
func (r *articlesRepoStub) CreateArticle(authorID, title, content string, tags []string) error {
	r.authorID, r.title, r.content, r.tags = authorID, title, content, tags
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
	id := uuid.New()
	repo := &articlesRepoStub{articles: []models.Article{{ID: id.String()}}, article: &models.Article{ID: id.String()}, incremented: make(chan uuid.UUID, 1)}
	svc := NewArticlesService(repo)

	articles, err := svc.GetArticles(3, 10, []string{"go"})
	if err != nil || len(articles) != 1 || repo.limit != 10 || repo.offset != 20 {
		t.Fatalf("GetArticles result=%+v call=%d/%d err=%v", articles, repo.limit, repo.offset, err)
	}
	a := models.Article{Title: "title", Content: "body", Tags: []string{"go"}, Author: models.Author{ID: "author"}}
	if err := svc.CreateArticle(a); err != nil || repo.authorID != "author" || repo.title != "title" {
		t.Fatalf("CreateArticle call not forwarded: %v %+v", err, repo)
	}
	got, err := svc.GetArticleWithID(id.String())
	if err != nil || got.ID != id.String() {
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
	if err := svc.UpdateArticle("author", id.String(), req); err != nil || repo.request.Title != "new" {
		t.Fatalf("UpdateArticle: %v", err)
	}
	if err := svc.DeleteArticle("author", id.String()); err != nil || repo.articleID != id {
		t.Fatalf("DeleteArticle: %v", err)
	}
}

func TestArticlesServiceErrors(t *testing.T) {
	repoErr := errors.New("repo error")
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
	if _, err := svc.GetArticleWithID(uuid.NewString()); !errors.Is(err, repoErr) {
		t.Fatalf("GetArticle error=%v", err)
	}
	if err := svc.UpdateArticle("a", "bad", models.UpdateArticleRequest{}); err == nil {
		t.Fatal("expected UUID parse error")
	}
	if err := svc.UpdateArticle("a", uuid.NewString(), models.UpdateArticleRequest{}); !errors.Is(err, repoErr) {
		t.Fatalf("Update error=%v", err)
	}
	if err := svc.DeleteArticle("a", "bad"); err == nil {
		t.Fatal("expected UUID parse error")
	}
	if err := svc.DeleteArticle("a", uuid.NewString()); !errors.Is(err, repoErr) {
		t.Fatalf("Delete error=%v", err)
	}
}

type interactionsRepoStub struct {
	comments        []models.Comment
	liked           bool
	count           int
	err             error
	articleID       uuid.UUID
	userID, content string
}

func (r *interactionsRepoStub) GetComments(id uuid.UUID) ([]models.Comment, error) {
	r.articleID = id
	return r.comments, r.err
}
func (r *interactionsRepoStub) CreateComment(id uuid.UUID, userID, content string) error {
	r.articleID, r.userID, r.content = id, userID, content
	return r.err
}
func (r *interactionsRepoStub) ToggleLike(id uuid.UUID, userID string) (bool, int, error) {
	r.articleID, r.userID = id, userID
	return r.liked, r.count, r.err
}

func TestInteractionsService(t *testing.T) {
	id := uuid.New()
	repo := &interactionsRepoStub{comments: []models.Comment{{ID: "c1"}}, liked: true, count: 2}
	svc := NewInteractionsService(repo)
	comments, err := svc.GetComments(id.String())
	if err != nil || len(comments) != 1 {
		t.Fatalf("GetComments=%+v,%v", comments, err)
	}
	if err := svc.CreateComment(id.String(), "u1", "hello"); err != nil || repo.content != "hello" {
		t.Fatalf("CreateComment=%v", err)
	}
	liked, count, err := svc.ToggleLike(id.String(), "u1")
	if err != nil || !liked || count != 2 {
		t.Fatalf("ToggleLike=%v,%d,%v", liked, count, err)
	}

	repo.err = errors.New("repo")
	if _, err := svc.GetComments("bad"); err == nil {
		t.Fatal("expected parse error")
	}
	if _, err := svc.GetComments(id.String()); err == nil {
		t.Fatal("expected repository error")
	}
	if err := svc.CreateComment("bad", "", ""); err == nil {
		t.Fatal("expected parse error")
	}
	if err := svc.CreateComment(id.String(), "", ""); err == nil {
		t.Fatal("expected repository error")
	}
	if _, _, err := svc.ToggleLike("bad", ""); err == nil {
		t.Fatal("expected parse error")
	}
	if _, _, err := svc.ToggleLike(id.String(), ""); err == nil {
		t.Fatal("expected repository error")
	}
}
