package handlers

import (
	"blog/internal/models"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func perform(method, path, pattern string, body any, userID any, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	r := gin.New()
	r.Handle(method, pattern, func(c *gin.Context) {
		if userID != nil {
			c.Set("userID", userID)
		}
		handler(c)
	})
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status=%d want=%d body=%s", w.Code, want, w.Body.String())
	}
}

type articlesServiceStub struct {
	articles            []models.Article
	article             *models.Article
	err                 error
	page, limit         int
	tags                []string
	created             models.Article
	authorID, articleID string
	updated             models.UpdateArticleRequest
}

func (s *articlesServiceStub) GetArticles(page, limit int, tags []string) ([]models.Article, error) {
	s.page, s.limit, s.tags = page, limit, tags
	return s.articles, s.err
}
func (s *articlesServiceStub) CreateArticle(a models.Article) error { s.created = a; return s.err }
func (s *articlesServiceStub) GetArticleWithID(id string) (*models.Article, error) {
	s.articleID = id
	return s.article, s.err
}
func (s *articlesServiceStub) UpdateArticle(userID, id string, r models.UpdateArticleRequest) error {
	s.authorID, s.articleID, s.updated = userID, id, r
	return s.err
}
func (s *articlesServiceStub) DeleteArticle(userID, id string) error {
	s.authorID, s.articleID = userID, id
	return s.err
}

func TestGetArticlesHandler(t *testing.T) {
	s := &articlesServiceStub{articles: []models.Article{{ID: "a1"}}}
	h := NewArticlesHandler(s)
	w := perform(http.MethodGet, "/articles?page=2&limit=10&tag=go&tag=api", "/articles", nil, nil, h.GetArticlesHandler)
	assertStatus(t, w, http.StatusOK)
	if s.page != 2 || s.limit != 10 || !reflect.DeepEqual(s.tags, []string{"go", "api"}) {
		t.Fatalf("unexpected parameters: %d %d %v", s.page, s.limit, s.tags)
	}

	w = perform(http.MethodGet, "/articles?page=no&limit=100", "/articles", nil, nil, h.GetArticlesHandler)
	assertStatus(t, w, http.StatusOK)
	if s.page != 1 || s.limit != 20 {
		t.Fatalf("defaults not applied: %d/%d", s.page, s.limit)
	}

	s.err = errors.New("db")
	w = perform(http.MethodGet, "/articles?page=0&limit=-1", "/articles", nil, nil, h.GetArticlesHandler)
	assertStatus(t, w, http.StatusInternalServerError)
}

func TestCreateArticleHandler(t *testing.T) {
	s := &articlesServiceStub{}
	h := NewArticlesHandler(s)
	body := models.Article{Title: "title", Content: "body", Tags: []string{"go"}}
	w := perform(http.MethodPost, "/articles", "/articles", body, "u1", h.CreateArticlesHandler)
	assertStatus(t, w, http.StatusCreated)
	if s.created.Author.ID != "u1" || s.created.Title != "title" {
		t.Fatalf("unexpected article: %+v", s.created)
	}

	cases := []struct {
		name   string
		raw    string
		user   any
		err    error
		status int
	}{
		{"bad json", `{`, "u1", nil, http.StatusBadRequest},
		{"missing user", `{"title":"x"}`, nil, nil, http.StatusInternalServerError},
		{"invalid user", `{"title":"x"}`, 12, nil, http.StatusBadRequest},
		{"service error", `{"title":"x"}`, "u1", errors.New("db"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s.err = tc.err
			r := gin.New()
			r.POST("/articles", func(c *gin.Context) {
				if tc.user != nil {
					c.Set("userID", tc.user)
				}
				h.CreateArticlesHandler(c)
			})
			req := httptest.NewRequest(http.MethodPost, "/articles", bytes.NewBufferString(tc.raw))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			assertStatus(t, w, tc.status)
		})
	}
}

func TestGetAndModifyArticleHandlers(t *testing.T) {
	s := &articlesServiceStub{article: &models.Article{ID: "a1"}}
	h := NewArticlesHandler(s)
	w := perform(http.MethodGet, "/articles/a1", "/articles/:id", nil, nil, h.GetArticleWithIDHandler)
	assertStatus(t, w, http.StatusOK)
	if s.articleID != "a1" {
		t.Fatalf("id=%q", s.articleID)
	}
	s.err = errors.New("db")
	w = perform(http.MethodGet, "/articles/a1", "/articles/:id", nil, nil, h.GetArticleWithIDHandler)
	assertStatus(t, w, http.StatusInternalServerError)

	s.err = nil
	req := models.UpdateArticleRequest{Title: "new", Content: "body", Tags: []string{"go"}}
	w = perform(http.MethodPut, "/articles/a1", "/articles/:id", req, "u1", h.UpdateArticleHandler)
	assertStatus(t, w, http.StatusOK)
	if s.authorID != "u1" || s.updated.Title != "new" {
		t.Fatalf("update not forwarded: %+v", s)
	}

	for _, tc := range []struct {
		name       string
		body       any
		user       any
		serviceErr error
		want       int
	}{
		{"bad body", map[string]any{"title": 1}, "u1", nil, 400},
		{"missing user", req, nil, nil, 500},
		{"bad user", req, 1, nil, 400},
		{"service error", req, "u1", errors.New("db"), 500},
	} {
		t.Run("update "+tc.name, func(t *testing.T) {
			s.err = tc.serviceErr
			assertStatus(t, perform(http.MethodPut, "/articles/a1", "/articles/:id", tc.body, tc.user, h.UpdateArticleHandler), tc.want)
		})
	}

	s.err = nil
	w = perform(http.MethodDelete, "/articles/a1", "/articles/:id", nil, "u1", h.DeleteArticleHandler)
	assertStatus(t, w, http.StatusOK)
	for _, tc := range []struct {
		name       string
		user       any
		serviceErr error
		want       int
	}{
		{"missing user", nil, nil, 500}, {"bad user", 1, nil, 400}, {"service error", "u1", errors.New("db"), 500},
	} {
		t.Run("delete "+tc.name, func(t *testing.T) {
			s.err = tc.serviceErr
			assertStatus(t, perform(http.MethodDelete, "/articles/a1", "/articles/:id", nil, tc.user, h.DeleteArticleHandler), tc.want)
		})
	}
}

type userServiceStub struct {
	err        error
	token      string
	registered *models.UserRegister
	loggedIn   *models.UserLogin
}

func (s *userServiceStub) Register(u *models.UserRegister) error { s.registered = u; return s.err }
func (s *userServiceStub) Login(u *models.UserLogin) (string, error) {
	s.loggedIn = u
	return s.token, s.err
}

func TestAuthHandlers(t *testing.T) {
	s := &userServiceStub{token: "jwt"}
	h := NewUserHandler(s)
	register := models.UserRegister{Email: "a@example.com", Username: "alice", Password: "secret"}
	assertStatus(t, perform(http.MethodPost, "/register", "/register", register, nil, h.RegisterUser), http.StatusCreated)
	if s.registered.Email != register.Email {
		t.Fatalf("register not forwarded: %+v", s.registered)
	}
	assertStatus(t, perform(http.MethodPost, "/register", "/register", map[string]any{"email": "bad"}, nil, h.RegisterUser), http.StatusBadRequest)
	s.err = errors.New("db")
	assertStatus(t, perform(http.MethodPost, "/register", "/register", register, nil, h.RegisterUser), http.StatusInternalServerError)

	s.err = nil
	login := models.UserLogin{Email: "a@example.com", Password: "secret"}
	w := perform(http.MethodPost, "/login", "/login", login, nil, h.LoginUser)
	assertStatus(t, w, http.StatusOK)
	if s.loggedIn.Email != login.Email || !bytes.Contains(w.Body.Bytes(), []byte(`"token":"jwt"`)) {
		t.Fatalf("unexpected login response: %s", w.Body.String())
	}
	assertStatus(t, perform(http.MethodPost, "/login", "/login", map[string]any{"email": "bad"}, nil, h.LoginUser), http.StatusBadRequest)
	s.err = errors.New("bad login")
	assertStatus(t, perform(http.MethodPost, "/login", "/login", login, nil, h.LoginUser), http.StatusInternalServerError)
}

type interactionsServiceStub struct {
	comments                   []models.Comment
	liked                      bool
	count                      int
	err                        error
	articleID, userID, content string
}

func (s *interactionsServiceStub) GetComments(id string) ([]models.Comment, error) {
	s.articleID = id
	return s.comments, s.err
}
func (s *interactionsServiceStub) CreateComment(id, user, content string) error {
	s.articleID, s.userID, s.content = id, user, content
	return s.err
}
func (s *interactionsServiceStub) ToggleLike(id, user string) (bool, int, error) {
	s.articleID, s.userID = id, user
	return s.liked, s.count, s.err
}

func TestInteractionsHandlers(t *testing.T) {
	s := &interactionsServiceStub{comments: []models.Comment{{ID: "c1"}}, liked: true, count: 3}
	h := NewInteractionsHandler(s)
	assertStatus(t, perform(http.MethodGet, "/articles/a1/comments", "/articles/:id/comments", nil, nil, h.GetCommentsHandler), 200)
	s.err = errors.New("db")
	assertStatus(t, perform(http.MethodGet, "/articles/a1/comments", "/articles/:id/comments", nil, nil, h.GetCommentsHandler), 500)

	s.err = nil
	body := models.CreateCommentRequest{Content: "hello"}
	assertStatus(t, perform(http.MethodPost, "/articles/a1/comments", "/articles/:id/comments", body, "u1", h.CreateCommentHandler), 201)
	if s.content != "hello" || s.userID != "u1" {
		t.Fatalf("comment not forwarded: %+v", s)
	}
	for _, tc := range []struct {
		name string
		body any
		user any
		err  error
		want int
	}{
		{"bad body", map[string]any{"content": 1}, "u1", nil, 400}, {"missing user", body, nil, nil, 500}, {"bad user", body, 1, nil, 400}, {"service error", body, "u1", errors.New("db"), 500},
	} {
		t.Run("comment "+tc.name, func(t *testing.T) {
			s.err = tc.err
			assertStatus(t, perform(http.MethodPost, "/articles/a1/comments", "/articles/:id/comments", tc.body, tc.user, h.CreateCommentHandler), tc.want)
		})
	}

	s.err = nil
	assertStatus(t, perform(http.MethodPost, "/articles/a1/like", "/articles/:id/like", nil, "u1", h.ToggleLikeHandler), 200)
	for _, tc := range []struct {
		name string
		user any
		err  error
		want int
	}{
		{"missing user", nil, nil, 500}, {"bad user", 1, nil, 400}, {"service error", "u1", errors.New("db"), 500},
	} {
		t.Run("like "+tc.name, func(t *testing.T) {
			s.err = tc.err
			assertStatus(t, perform(http.MethodPost, "/articles/a1/like", "/articles/:id/like", nil, tc.user, h.ToggleLikeHandler), tc.want)
		})
	}
}
