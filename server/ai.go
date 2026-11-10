// Разговор с моделью из приложения: чат и разбор входящих.
//
// Ходит через подключения из server/connections.go — у каждого свой адрес
// и ключ. Сеть юнита ограничена петлёй, поэтому наружу ходит только
// подключение «Шлюз (локальный)» (gateway/main.go на 127.0.0.1:18789);
// прямые внешние адреса ответят, лишь если юнит ослабить.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
)

// aiMsg — сообщение чата в формате OpenAI: им же и уходит провайдеру.
type aiMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

const (
	// aiSystem — базовый настрой: чьё хранилище, как отвечать.
	aiSystem = "Ты — помощник, встроенный в личное хранилище знаний Pillar Vault " +
		"(markdown-карточки: дела, заметки, цели). Отвечай по-русски, коротко и по делу, " +
		"без оговорок и повторов вопроса. Формат — простой текст, списки — дефисами."
	aiMaxMessages = 100      // история чата длиннее — обрезок контекста, а не память
	aiMaxBytes    = 64 << 10 // суммарный вес сообщений
	aiMaxCard     = 24 << 10 // карточка в контекст — не целиком, если огромная
)

// aiPickConnection — подключение по id; пустой id — первое в списке
// (дефолт для разбора входящих и забытого переключателя).
func aiPickConnection(list []connection, id string) (connection, error) {
	if len(list) == 0 {
		return connection{}, errors.New("нет ни одного подключения — добавь шлюз на экране «Шлюзы»")
	}
	if id == "" {
		return list[0], nil
	}
	if c, ok := findConnection(list, id); ok {
		return c, nil
	}
	return connection{}, errors.New("нет такого подключения")
}

// aiChat — многоходовый chat/completions. Рассуждающие модели (gpt-oss)
// при малом лимите сжигают всё на reasoning, поэтому лимит повышен,
// а пустой content подменяется ходом мысли.
func aiChat(c connection, msgs []aiMsg) (string, error) {
	payload := map[string]any{
		"model":      c.Model,
		"max_tokens": 2048,
		"messages":   msgs,
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost,
		strings.TrimRight(c.BaseURL, "/")+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	resp, err := connHTTP.Do(req)
	if err != nil {
		return "", probeErr(c, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", resp.Status, connShort(body))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				Reasoning string `json:"reasoning"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &out); err != nil || len(out.Choices) == 0 {
		return "", fmt.Errorf("ответ без choices: %s", connShort(body))
	}
	reply := strings.TrimSpace(out.Choices[0].Message.Content)
	if reply == "" {
		reply = strings.TrimSpace(out.Choices[0].Message.Reasoning)
	}
	if reply == "" {
		return "", errors.New("модель вернула пустой ответ")
	}
	return reply, nil
}

// aiReply — ответ чата приложению.
type aiReply struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Model   string `json:"model,omitempty"`
	Conn    string `json:"conn,omitempty"`
}

// aiJSON — ответ JSON-ом, без редиректа: приложение ждёт данные на месте.
func aiJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	if code != http.StatusOK {
		w.WriteHeader(code)
	}
	json.NewEncoder(w).Encode(v)
}

// handleAIChat — POST /api/ai/chat. Форма: conn (id, пусто — первое),
// messages (JSON-массив [{role,content}]), необязательный card — путь .md,
// чьё содержимое уходит контекстом. История живёт в браузере, сервер
// ничего не хранит.
func handleAIChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	if !widgetAuth(w, r) {
		return
	}
	fail := func(code int, msg string) { aiJSON(w, code, aiReply{OK: false, Message: msg}) }

	connMu.Lock()
	list, err := loadConnections()
	connMu.Unlock()
	if err != nil {
		fail(http.StatusInternalServerError, err.Error())
		return
	}
	c, err := aiPickConnection(list, strings.TrimSpace(r.FormValue("conn")))
	if err != nil {
		fail(http.StatusBadRequest, err.Error())
		return
	}

	var msgs []aiMsg
	raw := strings.TrimSpace(r.FormValue("messages"))
	if raw == "" || len(raw) > aiMaxBytes {
		fail(http.StatusBadRequest, "пустая или слишком длинная история")
		return
	}
	if err := json.Unmarshal([]byte(raw), &msgs); err != nil || len(msgs) == 0 {
		fail(http.StatusBadRequest, "messages — не JSON-массив сообщений")
		return
	}
	if len(msgs) > aiMaxMessages {
		msgs = msgs[len(msgs)-aiMaxMessages:]
	}
	for _, m := range msgs {
		if m.Role != "user" && m.Role != "assistant" {
			fail(http.StatusBadRequest, "роль только user или assistant")
			return
		}
	}

	system := aiSystem
	if card := strings.TrimSpace(r.FormValue("card")); card != "" {
		text, err := readCard(card)
		if err != nil {
			fail(http.StatusBadRequest, "карточка не читается: "+err.Error())
			return
		}
		if len(text) > aiMaxCard {
			text = string([]rune(text)[:aiMaxCard]) + "\n…(обрезано)"
		}
		system += "\n\nКонтекст — карточка «" + card + "»:\n" + text
	}
	msgs = append([]aiMsg{{Role: "system", Content: system}}, msgs...)

	reply, err := aiChat(c, msgs)
	if err != nil {
		fail(http.StatusBadGateway, err.Error())
		return
	}
	aiJSON(w, http.StatusOK, aiReply{OK: true, Message: reply, Model: c.Model, Conn: c.Name})
}

// aiInboxReply — разбор одной мысли: куда её и под каким названием.
type aiInboxReply struct {
	OK      bool   `json:"ok"`
	Action  string `json:"action"` // task | card | drop
	Title   string `json:"title"`
	Why     string `json:"why,omitempty"`
	Message string `json:"message,omitempty"` // причина отказа
}

// parseAIInbox достаёт вердикт из ответа модели. Модели любят обернуть JSON
// в слова — берём от первой скобки до последней. Совсем непонятное — не
// ошибка, а осторожная карточка с названием из самой мысли.
func parseAIInbox(reply, text string) aiInboxReply {
	fallback := aiInboxReply{OK: true, Action: "card", Title: aiShortTitle(text), Why: "ответ модели не распарсился"}
	start := strings.Index(reply, "{")
	end := strings.LastIndex(reply, "}")
	if start < 0 || end <= start {
		return fallback
	}
	var got struct {
		Action string `json:"action"`
		Title  string `json:"title"`
		Why    string `json:"why"`
	}
	if err := json.Unmarshal([]byte(reply[start:end+1]), &got); err != nil {
		return fallback
	}
	switch got.Action {
	case "task", "card", "drop":
	default:
		return fallback
	}
	got.Title = strings.TrimSpace(got.Title)
	if got.Title == "" {
		got.Title = aiShortTitle(text)
	}
	return aiInboxReply{OK: true, Action: got.Action, Title: got.Title, Why: strings.TrimSpace(got.Why)}
}

// aiShortTitle — мысль в название: первая строка, до 60 рун.
func aiShortTitle(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	if utf8.RuneCountInString(line) > 60 {
		line = string([]rune(line)[:60]) + "…"
	}
	return line
}

// handleAIInbox — POST /api/ai/inbox. Форма: conn (пусто — первое), text
// (мысль из входящих). Ответ — вердикт: дело, карточка или мусор.
func handleAIInbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	if !widgetAuth(w, r) {
		return
	}
	fail := func(code int, msg string) { aiJSON(w, code, aiInboxReply{OK: false, Message: msg}) }

	text := strings.TrimSpace(r.FormValue("text"))
	if text == "" || len(text) > 8<<10 {
		fail(http.StatusBadRequest, "пустой или слишком длинный текст")
		return
	}
	connMu.Lock()
	list, err := loadConnections()
	connMu.Unlock()
	if err != nil {
		fail(http.StatusInternalServerError, err.Error())
		return
	}
	c, err := aiPickConnection(list, strings.TrimSpace(r.FormValue("conn")))
	if err != nil {
		fail(http.StatusBadRequest, err.Error())
		return
	}

	msgs := []aiMsg{
		{Role: "system", Content: "Ты разбираешь входящие мысли личного хранилища знаний. " +
			"Реши, что делать с мыслью, и ответь СТРОГО одним JSON без пояснений и разметки: " +
			`{"action":"task|card|drop","title":"короткое название до 60 символов","why":"одна фраза почему"}. ` +
			"task — действие или дело (сделать, купить, позвонить, дедлайн); " +
			"card — знание на хранение (факт, идея, заметка, ссылка); " +
			"drop — шум: обрывок, повтор, ничего не значит."},
		{Role: "user", Content: text},
	}
	reply, err := aiChat(c, msgs)
	if err != nil {
		fail(http.StatusBadGateway, err.Error())
		return
	}
	aiJSON(w, http.StatusOK, parseAIInbox(reply, text))
}
