package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

const authCookieName = "quizdock_session"

type authManager struct {
	username string
	password [sha256.Size]byte
	mu       sync.Mutex
	sessions map[string]time.Time
}

func newAuthManager(username, password string) *authManager {
	manager := &authManager{username: username, sessions: make(map[string]time.Time)}
	if password != "" {
		manager.password = sha256.Sum256([]byte(password))
		if manager.username == "" {
			manager.username = "admin"
		}
	}
	return manager
}

func (a *authManager) enabled() bool {
	return a.password != [sha256.Size]byte{}
}

func (a *authManager) validCredentials(username, password string) bool {
	if !a.enabled() {
		return true
	}
	providedUser := sha256.Sum256([]byte(username))
	expectedUser := sha256.Sum256([]byte(a.username))
	providedPassword := sha256.Sum256([]byte(password))
	return subtle.ConstantTimeCompare(providedUser[:], expectedUser[:]) == 1 &&
		subtle.ConstantTimeCompare(providedPassword[:], a.password[:]) == 1
}

func (a *authManager) createSession() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buffer)
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for value, expires := range a.sessions {
		if expires.Before(now) {
			delete(a.sessions, value)
		}
	}
	a.sessions[token] = now.Add(7 * 24 * time.Hour)
	return token, nil
}

func (a *authManager) authenticated(request *http.Request) bool {
	if !a.enabled() {
		return true
	}
	cookie, err := request.Cookie(authCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	expires, ok := a.sessions[cookie.Value]
	if !ok || expires.Before(time.Now()) {
		delete(a.sessions, cookie.Value)
		return false
	}
	return true
}

func (a *authManager) removeSession(request *http.Request) {
	cookie, err := request.Cookie(authCookieName)
	if err != nil {
		return
	}
	a.mu.Lock()
	delete(a.sessions, cookie.Value)
	a.mu.Unlock()
}

func secureRequest(request *http.Request) bool {
	return request.TLS != nil || request.Header.Get("X-Forwarded-Proto") == "https"
}

func setSessionCookie(writer http.ResponseWriter, request *http.Request, token string) {
	http.SetCookie(writer, &http.Cookie{
		Name: authCookieName, Value: token, Path: "/", HttpOnly: true,
		Secure: secureRequest(request), SameSite: http.SameSiteStrictMode,
		MaxAge: int((7 * 24 * time.Hour).Seconds()),
	})
}

func clearSessionCookie(writer http.ResponseWriter, request *http.Request) {
	http.SetCookie(writer, &http.Cookie{
		Name: authCookieName, Value: "", Path: "/", HttpOnly: true,
		Secure: secureRequest(request), SameSite: http.SameSiteStrictMode,
		MaxAge: -1,
	})
}
