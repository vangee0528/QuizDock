package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthManagerSessionLifecycle(t *testing.T) {
	auth := newAuthManager("tester", "correct-password")
	if !auth.enabled() || auth.validCredentials("tester", "wrong") {
		t.Fatal("credential validation failed")
	}
	if !auth.validCredentials("tester", "correct-password") {
		t.Fatal("valid credentials were rejected")
	}
	token, err := auth.createSession()
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil)
	request.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	if !auth.authenticated(request) {
		t.Fatal("valid session was rejected")
	}
	auth.removeSession(request)
	if auth.authenticated(request) {
		t.Fatal("removed session remained valid")
	}
}

func TestAuthDisabledWithoutPassword(t *testing.T) {
	auth := newAuthManager("", "")
	if auth.enabled() {
		t.Fatal("authentication should be disabled")
	}
	if !auth.authenticated(httptest.NewRequest(http.MethodGet, "/", nil)) {
		t.Fatal("disabled authentication should allow requests")
	}
}
