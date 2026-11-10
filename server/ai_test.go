// Чат и разбор входящих: формат ответа провайдера и разбор вердикта модели.
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// aiStub — подставной OpenAI-совместимый провайдер: отдаёт заранее
// заданное тело и запоминает, что спросили.
func aiStub(t *testing.T, body string) (*httptest.Server, *map[string]any) {
	t.Helper()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestAIChatОтдаётОтвет(t *testing.T) {
	srv, got := aiStub(t, `{"choices":[{"message":{"content":"привет из модели"}}]}`)
	c := connection{BaseURL: srv.URL, Model: "test-model"}
	reply, err := aiChat(c, []aiMsg{{Role: "user", Content: "привет"}})
	if err != nil {
		t.Fatalf("aiChat: %v", err)
	}
	if reply != "привет из модели" {
		t.Fatalf("ответ: %q", reply)
	}
	if (*got)["model"] != "test-model" {
		t.Fatalf("модель ушла не та: %v", (*got)["model"])
	}
	msgs, _ := (*got)["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("сообщений ушло %d, ждали 1", len(msgs))
	}
}

func TestAIChatХодМыслиВместоПустогоОтвета(t *testing.T) {
	srv, _ := aiStub(t, `{"choices":[{"message":{"content":"","reasoning":"сначала подумал"}}]}`)
	c := connection{BaseURL: srv.URL, Model: "m"}
	reply, err := aiChat(c, []aiMsg{{Role: "user", Content: "x"}})
	if err != nil {
		t.Fatalf("aiChat: %v", err)
	}
	if reply != "сначала подумал" {
		t.Fatalf("ждали reasoning, ответ: %q", reply)
	}
}

func TestAIChatОшибкаПровайдера(t *testing.T) {
	srv, _ := aiStub(t, `{"choices":[]}`)
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	})
	c := connection{BaseURL: srv.URL, Model: "m"}
	if _, err := aiChat(c, []aiMsg{{Role: "user", Content: "x"}}); err == nil {
		t.Fatal("ждали ошибку на 401 провайдера")
	}
}

func TestAIPickConnection(t *testing.T) {
	list := []connection{{ID: "a", Name: "первое"}, {ID: "b", Name: "второе"}}
	if c, err := aiPickConnection(list, ""); err != nil || c.ID != "a" {
		t.Fatalf("дефолт — первое: %+v %v", c, err)
	}
	if c, err := aiPickConnection(list, "b"); err != nil || c.Name != "второе" {
		t.Fatalf("по id: %+v %v", c, err)
	}
	if _, err := aiPickConnection(list, "zzz"); err == nil {
		t.Fatal("неизвестный id — ошибка")
	}
	if _, err := aiPickConnection(nil, ""); err == nil {
		t.Fatal("пустой список — ошибка")
	}
}

func TestParseAIInbox(t *testing.T) {
	cases := []struct {
		name   string
		reply  string
		action string
		title  string
	}{
		{"чистый JSON", `{"action":"task","title":"Купить молоко","why":"дело"}`, "task", "Купить молоко"},
		{"JSON в словах", `Вот мой ответ: {"action":"card","title":"Факт о тройках"} — надеюсь, помог`, "card", "Факт о тройках"},
		{"дроп", `{"action":"drop","title":"шум"}`, "drop", "шум"},
	}
	for _, tc := range cases {
		got := parseAIInbox(tc.reply, "какая-то мысль")
		if !got.OK || got.Action != tc.action || got.Title != tc.title {
			t.Fatalf("%s: %+v", tc.name, got)
		}
	}
	// мусорный ответ — осторожная карточка с названием из мысли
	got := parseAIInbox("ничего не понял", "мысль про тройки")
	if !got.OK || got.Action != "card" || !strings.HasPrefix(got.Title, "мысль про тройки") {
		t.Fatalf("fallback: %+v", got)
	}
	// неизвестное действие — тоже fallback
	got = parseAIInbox(`{"action":"burn","title":"x"}`, "мысль")
	if got.Action != "card" {
		t.Fatalf("неизвестное действие: %+v", got)
	}
}
