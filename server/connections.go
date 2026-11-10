// Подключения к LLM-провайдерам (OpenAI-совместимые, через шлюз — любые).
//
// Список лежит в server/.connections.json (0600, вне git — как .passhash).
// Ключ наружу не уходит: GET /api/connections отдаёт только хвост (последние
// 4 символа). Проверка — живой запрос к провайдеру с машины сервера.
//
// ⚠ Сеть юнита. pillar-vault.service ограничен петлёй (IPAddressDeny=any,
// IPAddressAllow=localhost): прямой вызов api.x.ai из процесса не пройдёт.
// Рабочий путь — свой локальный шлюз на петле (127.0.0.1:18789,
// gateway/main.go): настройка в deploy/GATEWAY.md. Запасной — ослабить юнит
// (разрешить исходящие наружу), но песочница при этом проседает.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ─────────────────── пресеты провайдеров ───────────────────

type connPreset struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`            // модель по умолчанию
	NoKey   bool   `json:"no_key,omitempty"` // локальный, ключ не нужен
}

// connPresets — провайдеры из коробки. Все — OpenAI-совместимые:
// один и тот же клиент (chat/completions) ходит к любому.
// «gateway» — наш локальный шлюз на петле (gateway/main.go): он держит
// ключи провайдеров у себя, приложению нужен только адрес.
var connPresets = []connPreset{
	{ID: "gateway", Label: "Шлюз (локальный)", BaseURL: "http://127.0.0.1:18789/v1", Model: "openai/gpt-oss-120b", NoKey: true},
	{ID: "groq", Label: "Groq", BaseURL: "https://api.groq.com/openai/v1", Model: "openai/gpt-oss-120b"},
	{ID: "grok", Label: "Grok (xAI)", BaseURL: "https://api.x.ai/v1", Model: "grok-4.7"},
	{ID: "openai", Label: "OpenAI", BaseURL: "https://api.openai.com/v1", Model: "gpt-5"},
	{ID: "openrouter", Label: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1", Model: "x-ai/grok-4.7"},
	{ID: "ollama", Label: "Ollama (локально)", BaseURL: "http://127.0.0.1:11434/v1", Model: "llama3.1", NoKey: true},
}

func presetByID(id string) (connPreset, bool) {
	for _, p := range connPresets {
		if p.ID == id {
			return p, true
		}
	}
	return connPreset{}, false
}

// ─────────────────── хранилище ───────────────────

type connection struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Preset  string `json:"preset"` // id из connPresets или "custom"
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	Key     string `json:"key"` // только на диске; в API — key_tail
	Created string `json:"created"`
}

// connectionOut — то, что видит приложение: ключа нет, есть хвост.
type connectionOut struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Preset  string `json:"preset"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	KeyTail string `json:"key_tail"`         // …ab12 или пусто
	NoKey   bool   `json:"no_key,omitempty"` // ключа нет — локальный
	Created string `json:"created"`
}

type connectionsFile struct {
	List []connection `json:"connections"`
}

var connMu sync.Mutex

func connectionsPath() string {
	return filepath.Join(root, "server", ".connections.json")
}

// loadConnections читает список с диска. Нет файла — пустой список.
func loadConnections() ([]connection, error) {
	b, err := os.ReadFile(connectionsPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f connectionsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("connections.json битый: %w", err)
	}
	return f.List, nil
}

// saveConnections пишет список атомарно (tmp + rename), права 0600.
func saveConnections(list []connection) error {
	b, err := json.MarshalIndent(connectionsFile{List: list}, "", "  ")
	if err != nil {
		return err
	}
	tmp := connectionsPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, connectionsPath())
}

// connOut прячет ключ, оставляя хвост — как у паролей в платёжных формах.
func connOut(c connection) connectionOut {
	tail := ""
	if len(c.Key) >= 4 {
		tail = c.Key[len(c.Key)-4:]
	}
	return connectionOut{ID: c.ID, Name: c.Name, Preset: c.Preset, BaseURL: c.BaseURL,
		Model: c.Model, KeyTail: tail, NoKey: c.Key == "", Created: c.Created}
}

// findConnection ищет по id; ключ возвращается как есть (для запросов).
func findConnection(list []connection, id string) (connection, bool) {
	for _, c := range list {
		if c.ID == id {
			return c, true
		}
	}
	return connection{}, false
}

// ─────────────────── клиент провайдера (OpenAI-совместимый) ───────────────────

var connHTTP = &http.Client{Timeout: 25 * time.Second}

// probeModels — лёгкая проверка: GET /models с ключом.
// Ключ валиден и сеть есть → список имён моделей.
func probeModels(c connection) ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(c.BaseURL, "/")+"/models", nil)
	if err != nil {
		return nil, err
	}
	if c.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	resp, err := connHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", resp.Status, connShort(body))
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("ответ не похож на список моделей: %w", err)
	}
	names := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		names = append(names, m.ID)
	}
	sort.Strings(names)
	return names, nil
}

// probeErr переводит сетевые ошибки проверки на человеческий язык.
// Главный случай: юнит сервера без выхода в интернет — внешний API
// уходит в таймаут, и сырой «context deadline exceeded» ничего не объясняет.
func probeErr(c connection, err error) error {
	msg := err.Error()
	host := c.BaseURL
	if u, e := url.Parse(c.BaseURL); e == nil && u.Host != "" {
		host = u.Host
	}
	loopback := strings.HasPrefix(host, "127.0.0.1") ||
		strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "[::1]")
	switch {
	case strings.Contains(msg, "connection refused"):
		if loopback {
			return fmt.Errorf("на %s никто не отвечает — для локального шлюза проверьте службу: systemctl status pillar-gateway", host)
		}
		return fmt.Errorf("%s отказал в соединении", host)
	case strings.Contains(msg, "no such host"):
		return fmt.Errorf("адрес не найден (DNS): %s", host)
	case strings.Contains(msg, "Client.Timeout"), strings.Contains(msg, "deadline exceeded"):
		if loopback {
			return fmt.Errorf("%s не ответил за %s", host, connHTTP.Timeout)
		}
		return fmt.Errorf("внешний API не ответил за %s — у сервера нет выхода в интернет (песочница юнита); рабочий путь — «Шлюз (локальный)», он держит ключи у себя", connHTTP.Timeout)
	}
	return err
}

// probeChat — глубокая проверка: короткий chat/completions к модели
// подключения. Возвращает ответ модели. У рассуждающих моделей (gpt-oss)
// лимит токенов повышен: иначе всё уходит в reasoning, ответ пустой.
func probeChat(c connection, prompt string) (string, error) {
	payload := map[string]any{
		"model":      c.Model,
		"max_tokens": 1024,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
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
		return "", err
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
		// рассуждающая модель: показываем ход мысли, если content пуст
		reply = strings.TrimSpace(out.Choices[0].Message.Reasoning)
	}
	return reply, nil
}

// checkConnURL — адрес провайдера: полный URL, причём http разрешён
// только на петле (свой шлюз); внешние — строго https.
func checkConnURL(baseURL string) error {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("адрес должен быть полным URL, например https://api.x.ai/v1")
	}
	if u.Scheme != "https" && u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" {
		return fmt.Errorf("http разрешён только на петле (127.0.0.1); внешние — https")
	}
	return nil
}

// connShort — кусок тела ошибки провайдера для сообщения пользователю.
func connShort(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// ─────────────────── HTTP ───────────────────

// handleConnectionsAPI — список для экрана «Подключения».
// Ключи замаскированы до хвоста; пресеты — для формы добавления.
func handleConnectionsAPI(w http.ResponseWriter, r *http.Request) {
	if !widgetAuth(w, r) {
		return
	}
	connMu.Lock()
	list, err := loadConnections()
	connMu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := struct {
		Connections []connectionOut `json:"connections"`
		Presets     []connPreset    `json:"presets"`
	}{Connections: []connectionOut{}, Presets: connPresets}
	for _, c := range list {
		out.Connections = append(out.Connections, connOut(c))
	}
	json.NewEncoder(w).Encode(out)
}

// handleConnections — записи: добавить, удалить, проверить.
// Приложение шлёт форму с X-Pillar-Api — apiForms превратит редирект в JSON.
// do=test отвечает JSON сразу (нет перехода, есть ответ модели).
// do=add с verify=1 сначала зовёт модель живым запросом: ключ, не прошедший
// проверку, в список не попадает (мастер первого подключения во вкладке «ИИ»).
func handleConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet { // экран «Подключения» — приложение
		handleRoot(w, r)
		return
	}
	if !requirePost(w, r) {
		return
	}
	fail := func(msg string) { http.Redirect(w, r, urlWithMsg("/connections", msg), http.StatusSeeOther) }

	switch r.FormValue("do") {
	case "add":
		presetID := strings.TrimSpace(r.FormValue("preset"))
		name := strings.TrimSpace(r.FormValue("name"))
		baseURL := strings.TrimSpace(r.FormValue("base_url"))
		model := strings.TrimSpace(r.FormValue("model"))
		key := strings.TrimSpace(r.FormValue("key"))

		if presetID == "custom" {
			if name == "" || baseURL == "" || model == "" {
				fail("Для своего провайдера нужны название, адрес и модель")
				return
			}
		} else {
			p, ok := presetByID(presetID)
			if !ok {
				fail("Неизвестный провайдер")
				return
			}
			if name == "" {
				name = p.Label
			}
			if baseURL == "" {
				baseURL = p.BaseURL
			}
			if model == "" {
				model = p.Model
			}
			if key == "" && !p.NoKey {
				fail("Нужен ключ API")
				return
			}
		}
		if err := checkConnURL(baseURL); err != nil {
			fail(err.Error())
			return
		}

		c := connection{
			ID:      newConnID(),
			Name:    name,
			Preset:  presetID,
			BaseURL: strings.TrimRight(baseURL, "/"),
			Model:   model,
			Key:     key,
			Created: time.Now().Format("2006-01-02 15:04"),
		}

		// verify=1 — глубокая проверка до сохранения: короткий разговор
		// с моделью. Не ответила — подключение не создаётся, а приложение
		// получает JSON с причиной (502), а не «добавлено».
		if r.FormValue("verify") == "1" {
			if _, err := probeChat(c, "Ответь одним словом: на связи"); err != nil {
				msg := probeErr(c, err).Error()
				if wantsAPI(r) {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.Header().Set("Cache-Control", "private, no-store")
					w.WriteHeader(http.StatusBadGateway)
					json.NewEncoder(w).Encode(apiResult{OK: false, Message: msg})
				} else {
					fail("Ключ не прошёл проверку: " + msg)
				}
				return
			}
		}

		connMu.Lock()
		list, err := loadConnections()
		if err == nil {
			list = append(list, c)
			err = saveConnections(list)
		}
		connMu.Unlock()
		if err != nil {
			fail("Не записалось: " + err.Error())
			return
		}
		markOK(w)
		http.Redirect(w, r, urlWithMsg("/connections", "Подключение добавлено"), http.StatusSeeOther)

	case "remove":
		id := r.FormValue("id")
		connMu.Lock()
		list, err := loadConnections()
		if err == nil {
			kept := list[:0]
			for _, c := range list {
				if c.ID != id {
					kept = append(kept, c)
				}
			}
			if len(kept) == len(list) {
				err = fmt.Errorf("нет такого подключения")
			} else {
				err = saveConnections(kept)
			}
		}
		connMu.Unlock()
		if err != nil {
			fail("Не удалилось: " + err.Error())
			return
		}
		markOK(w)
		http.Redirect(w, r, urlWithMsg("/connections", "Подключение удалено"), http.StatusSeeOther)

	case "update":
		// Правка энтри: название, адрес, модель. Пустой ключ — не менять
		// (в форме он «без изменений»), заданный — заменить.
		id := r.FormValue("id")
		name := strings.TrimSpace(r.FormValue("name"))
		baseURL := strings.TrimSpace(r.FormValue("base_url"))
		model := strings.TrimSpace(r.FormValue("model"))
		key := strings.TrimSpace(r.FormValue("key"))
		if name == "" || baseURL == "" || model == "" {
			fail("Нужны название, адрес и модель")
			return
		}
		if err := checkConnURL(baseURL); err != nil {
			fail(err.Error())
			return
		}
		connMu.Lock()
		list, err := loadConnections()
		if err == nil {
			err = fmt.Errorf("нет такого подключения")
			for i := range list {
				if list[i].ID == id {
					list[i].Name = name
					list[i].BaseURL = strings.TrimRight(baseURL, "/")
					list[i].Model = model
					if key != "" {
						list[i].Key = key
					}
					err = saveConnections(list)
					break
				}
			}
		}
		connMu.Unlock()
		if err != nil {
			fail("Не сохранилось: " + err.Error())
			return
		}
		markOK(w)
		http.Redirect(w, r, urlWithMsg("/connections", "Подключение обновлено"), http.StatusSeeOther)

	case "test":
		// Живой запрос к провайдеру. Отвечаем JSON сразу, без редиректа:
		// ответ модели нужно показать на месте.
		if !wantsAPI(r) {
			http.Error(w, "проверка только из приложения", http.StatusBadRequest)
			return
		}
		id := r.FormValue("id")
		prompt := strings.TrimSpace(r.FormValue("prompt"))
		connMu.Lock()
		list, err := loadConnections()
		connMu.Unlock()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		c, ok := findConnection(list, id)
		if !ok {
			http.Error(w, "нет такого подключения", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "private, no-store")
		if prompt == "" {
			// Лёгкая проверка: ключ + список моделей.
			names, err := probeModels(c)
			if err != nil {
				w.WriteHeader(http.StatusBadGateway)
				json.NewEncoder(w).Encode(apiResult{OK: false, Message: probeErr(c, err).Error()})
				return
			}
			show := names
			if len(show) > 5 {
				show = show[:5]
			}
			what := "Ключ принят"
			if c.Key == "" {
				what = "Соединение есть"
			}
			json.NewEncoder(w).Encode(apiResult{OK: true,
				Message: fmt.Sprintf("%s. Моделей: %d. Первые: %s", what, len(names), strings.Join(show, ", "))})
			return
		}
		// Глубокая проверка: короткий разговор с моделью подключения.
		reply, err := probeChat(c, prompt)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(apiResult{OK: false, Message: probeErr(c, err).Error()})
			return
		}
		json.NewEncoder(w).Encode(apiResult{OK: true, Message: reply})

	default:
		fail("Неизвестное действие")
	}
}

// newConnID — короткий уникальный id подключения.
func newConnID() string {
	return fmt.Sprintf("c%d", time.Now().UnixNano())
}
