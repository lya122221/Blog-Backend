package handlers

import (
	"analytics/internal/models"
	"analytics/internal/services"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/gin-gonic/gin"
)

type viewsServiceStub struct {
	request   models.ViewRequest
	visitorID string
	calls     int
	err       error
}

func (stub *viewsServiceStub) RecordViews(_ context.Context, request models.ViewRequest, visitorID string) error {
	stub.request = request
	stub.visitorID = visitorID
	stub.calls++
	return stub.err
}

func newTestHandler(stub *viewsServiceStub) *ViewsHandler {
	return NewViewsHandler(stub, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), true)
}

func sendViews(handler *ViewsHandler, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/analytics/views", handler.Views)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/analytics/views", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func validBody() string {
	request := models.ViewRequest{Events: []models.ViewEvent{{
		ID: uuid.New().String(), Type: models.ArticleOpened, ArticleID: uuid.New().String(),
		OccurredAt: time.Now().UTC(),
	}}}
	data, _ := json.Marshal(request)
	return string(data)
}

func TestViewsPassesRequestAndReusesVisitorCookie(t *testing.T) {
	stub := &viewsServiceStub{}
	handler := newTestHandler(stub)
	first := sendViews(handler, validBody(), nil)
	if first.Code != http.StatusAccepted || stub.calls != 1 || len(stub.request.Events) != 1 {
		t.Fatalf("first response = %d, service calls = %d", first.Code, stub.calls)
	}
	cookies := first.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != cookieName || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("visitor cookie = %+v", cookies)
	}
	if stub.visitorID != cookies[0].Value {
		t.Fatalf("service visitor ID = %q, cookie = %q", stub.visitorID, cookies[0].Value)
	}
	second := sendViews(handler, validBody(), cookies[0])
	if second.Code != http.StatusAccepted || len(second.Result().Cookies()) != 0 || stub.visitorID != cookies[0].Value || stub.calls != 2 {
		t.Fatalf("cookie not reused: code=%d, visitor=%q, calls=%d", second.Code, stub.visitorID, stub.calls)
	}
}

func TestViewsReplacesInvalidCookie(t *testing.T) {
	stub := &viewsServiceStub{}
	response := sendViews(newTestHandler(stub), validBody(), &http.Cookie{Name: cookieName, Value: "bad"})
	if response.Code != http.StatusAccepted || len(response.Result().Cookies()) != 1 || stub.visitorID == "bad" {
		t.Fatalf("response = %d, cookies = %+v", response.Code, response.Result().Cookies())
	}
}

func TestViewsRejectsMalformedJSONBeforeService(t *testing.T) {
	for name, body := range map[string]string{
		"bad JSON":       "{",
		"unknown field":  `{"events":[],"visitor_id":"forged"}`,
		"trailing JSON":  validBody() + `{}`,
		"oversized body": strings.Repeat(" ", maxBodyBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			stub := &viewsServiceStub{}
			response := sendViews(newTestHandler(stub), body, nil)
			if response.Code != http.StatusBadRequest || stub.calls != 0 {
				t.Fatalf("response = %d, service calls = %d", response.Code, stub.calls)
			}
		})
	}
}

func TestViewsMapsServiceErrors(t *testing.T) {
	for name, test := range map[string]struct {
		err        error
		wantStatus int
	}{
		"invalid event": {err: &services.ValidationError{}, wantStatus: http.StatusBadRequest},
		"broker error":  {err: errors.New("broker unavailable"), wantStatus: http.StatusServiceUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			stub := &viewsServiceStub{err: test.err}
			response := sendViews(newTestHandler(stub), validBody(), nil)
			if response.Code != test.wantStatus || stub.calls != 1 {
				t.Fatalf("response = %d, service calls = %d", response.Code, stub.calls)
			}
			if test.wantStatus == http.StatusServiceUnavailable && response.Header().Get("Retry-After") != "1" {
				t.Fatal("retry header missing")
			}
		})
	}
}
