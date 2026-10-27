package route

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/becaraya/katana-api/internal/bootstrap"
	"github.com/gin-gonic/gin"
)

type api struct {
	t *testing.T
	r *gin.Engine
}

func newAPI(t *testing.T) *api {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Setup(&bootstrap.Env{AppEnv: "development", AccessTokenSecret: "test-secret", AccessTokenExpiryHour: 1}, time.Second, r)
	return &api{t: t, r: r}
}

func (a *api) do(method, path, token string, body any) (int, map[string]any) {
	a.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	rec := httptest.NewRecorder()
	a.r.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (a *api) login(name string) string {
	a.t.Helper()
	code, out := a.do("POST", "/login", "", map[string]string{"username": name})
	if code != http.StatusOK {
		a.t.Fatalf("login %s: %d %v", name, code, out)
	}
	return out["token"].(string)
}

func TestRoutesRequireAuthentication(t *testing.T) {
	a := newAPI(t)
	for _, path := range []string{"/game", "/game/me", "/catalog"} {
		if code, _ := a.do("GET", path, "", nil); code != http.StatusUnauthorized {
			t.Errorf("GET %s without token: %d", path, code)
		}
	}
}

func TestLobbyAndGameFlow(t *testing.T) {
	a := newAPI(t)
	alice, bob := a.login("alice-route"), a.login("bob-route")
	defer a.do("DELETE", "/debug/game", alice, nil)

	if code, out := a.do("GET", "/catalog", alice, nil); code != http.StatusOK || len(out["cards"].([]any)) == 0 || len(out["characters"].([]any)) == 0 {
		t.Fatalf("catalog: %d", code)
	}

	if code, _ := a.do("POST", "/debug/game?players=3&start=true", alice, nil); code != http.StatusOK {
		t.Fatalf("debug game: %d", code)
	}
	if _, out := a.do("GET", "/game", alice, nil); out["game"].(map[string]any)["state"] != "STARTED" {
		t.Fatalf("game should be started: %v", out["game"])
	}

	// Une partie lancée n'accepte plus de nouveau joueur.
	if code, out := a.do("POST", "/game/join", bob, nil); code != http.StatusConflict || out["code"] != "GAME_IN_PROGRESS" {
		t.Errorf("join during game: %d %v", code, out)
	}

	// La main reste privée, et une requête périmée ne joue jamais une autre carte.
	_, me := a.do("GET", "/game/me", alice, nil)
	if len(me["hand"].([]any)) == 0 {
		t.Fatal("alice should hold cards")
	}
	if code, out := a.do("POST", "/game/play", alice, map[string]any{"card": 0, "card_id": "carte-inexistante"}); code != http.StatusNotFound || out["code"] != "CARD_NOT_FOUND" {
		t.Errorf("stale card: %d %v", code, out)
	}
	if code, out := a.do("POST", "/game/respond", alice, map[string]any{"parry": true, "card": 0, "card_id": "carte-inexistante"}); code != http.StatusNotFound {
		t.Errorf("stale parry: %d %v", code, out)
	}

	// Un spectateur n'agit pas.
	if code, _ := a.do("POST", "/game/turn/end", bob, nil); code != http.StatusForbidden && code != http.StatusNotFound {
		t.Errorf("spectator end turn: %d", code)
	}
	if code, out := a.do("POST", "/game/chat", bob, map[string]string{"text": "salut"}); code != http.StatusForbidden || out["code"] != "NOT_IN_GAME" {
		t.Errorf("spectator chat: %d %v", code, out)
	}
}

func TestChatValidationAndRateLimit(t *testing.T) {
	a := newAPI(t)
	alice := a.login("alice-chat")
	defer a.do("DELETE", "/debug/game", alice, nil)
	a.do("POST", "/debug/game?players=3&start=false", alice, nil)

	if code, _ := a.do("POST", "/game/chat", alice, map[string]string{"text": "   "}); code != http.StatusBadRequest {
		t.Errorf("empty message: %d", code)
	}
	if code, _ := a.do("POST", "/game/chat", alice, map[string]string{"text": "Bonjour la table"}); code != http.StatusOK {
		t.Fatalf("first message: %d", code)
	}
	if code, out := a.do("POST", "/game/chat", alice, map[string]string{"text": "encore"}); code != http.StatusTooManyRequests || out["code"] != "CHAT_RATE" {
		t.Errorf("burst: %d %v", code, out)
	}
	_, out := a.do("GET", "/game", alice, nil)
	if chat := out["game"].(map[string]any)["chat"].([]any); len(chat) != 1 {
		t.Errorf("chat history: %v", chat)
	}
}
