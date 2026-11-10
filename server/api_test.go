// API React-приложения: формы отвечают JSON, а граница доступа та же —
// гость не получает ни приложения, ни данных, ни права записи.
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// apiPost — форма от приложения: с X-Pillar-Api и через ту же обёртку, что в main.
func apiPost(h http.HandlerFunc, path string, form url.Values, admin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set(apiHeader, "1")
	if admin {
		req = asAdmin(req)
	}
	rec := httptest.NewRecorder()
	apiForms(h).ServeHTTP(rec, req)
	return rec
}

func apiResultOf(t *testing.T, rec *httptest.ResponseRecorder) apiResult {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("ответ не JSON (%s, код %d): %s", ct, rec.Code, rec.Body)
	}
	if rec.Header().Get("Location") != "" || rec.Header().Get(okHeader) != "" {
		t.Errorf("служебные заголовки утекли в ответ: %v", rec.Header())
	}
	var res apiResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("битый JSON: %v: %s", err, rec.Body)
	}
	return res
}

func TestAPIФормыОтвечаютJSON(t *testing.T) {
	dir := withAccess(t, "ключ-для-теста")

	// гостю — 401, а не редирект: приложение поймёт, что пора на форму входа
	if rec := apiPost(handleVow, "/vow", url.Values{"name": {"Сон"}, "mark": {"✓"}}, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("гость записал столп: код %d", rec.Code)
	}

	// успех без сообщения
	rec := apiPost(handleVow, "/vow", url.Values{"name": {"Сон"}, "mark": {"✓"}}, true)
	if res := apiResultOf(t, rec); !res.OK || rec.Code != http.StatusOK {
		t.Fatalf("столп: %+v, код %d", res, rec.Code)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "pillars", "daily-vows.md"))
	if !strings.Contains(string(raw), "✓ |") {
		t.Error("отметка не дошла до карточки")
	}

	// отказ: сообщение обработчика и 422
	rec = apiPost(handleToday, "/today", url.Values{"do": {"done"}, "text": {"нет такого"}}, true)
	if res := apiResultOf(t, rec); res.OK || res.Message == "" || rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("отказ принят за успех: %+v, код %d", res, rec.Code)
	}

	// успех с сообщением («записано») — помечен обработчиком и не считается отказом
	rec = apiPost(handleDiary, "/diary", url.Values{"text": {"запись из приложения"}}, true)
	if res := apiResultOf(t, rec); !res.OK || res.Message != "записано" {
		t.Fatalf("дневник: %+v, код %d", res, rec.Code)
	}

	// без X-Pillar-Api — прежний редирект для старого интерфейса
	if rec := postForm(handleVow, "/vow", url.Values{"name": {"Сон"}, "mark": {"✓"}}, true); rec.Code != http.StatusSeeOther {
		t.Errorf("старый интерфейс получил %d вместо редиректа", rec.Code)
	}
}

func TestAPIПриложениеИМенюТолькоПоСессии(t *testing.T) {
	dir := withAccess(t, "ключ-для-теста")
	write := func(rel, body string) {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(body), 0o644)
	}
	write("viewer/x/index.html", "<!doctype html><div id=root></div>")
	write("viewer/x/assets/app-1234.js", "console.log('приложение')")

	get := func(path string, admin bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if admin {
			req = asAdmin(req)
		}
		rec := httptest.NewRecorder()
		mux := http.NewServeMux()
		mux.HandleFunc("/", handleRoot)
		mux.HandleFunc("/api/nav", handleNav)
		mux.ServeHTTP(rec, req)
		return rec
	}

	for _, p := range []string{"/assets/app-1234.js", "/doc/любой/маршрут", "/api/nav"} {
		rec := get(p, false)
		if rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "приложение") || strings.Contains(rec.Body.String(), "dirs") {
			t.Errorf("гость получил %s: код %d", p, rec.Code)
		}
	}
	if rec := get("/doc/любой/маршрут", true); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "root") {
		t.Errorf("маршрут приложения не отдал index.html: %d", rec.Code)
	}
	if rec := get("/assets/app-1234.js", true); !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("файл сборки с хэшем не кэшируется: %q", rec.Header().Get("Cache-Control"))
	}
	if rec := get("/", true); !strings.Contains(rec.Header().Get("Cache-Control"), "no-cache") {
		t.Errorf("index.html кэшируется — после выкладки грузилась бы старая сборка")
	}
	for _, p := range []string{"/assets/../SOUL.md", "/assets/../../SOUL.md"} {
		if rec := get(p, true); strings.Contains(rec.Body.String(), "устав") {
			t.Errorf("через %s видно корень хранилища", p)
		}
	}
	rec := get("/api/nav", true)
	var nav navData
	if err := json.Unmarshal(rec.Body.Bytes(), &nav); err != nil || nav.Cards == 0 || len(nav.Dirs) == 0 {
		t.Errorf("меню пустое: %v %s", err, rec.Body)
	}
}

func TestВходВозвращаетНаЭкран(t *testing.T) {
	withAccess(t, "ключ-для-теста")
	login := func(next string) string {
		rec := postForm(handleLogin, "/login", url.Values{"password": {"ключ-для-теста"}, "next": {next}}, false)
		return rec.Header().Get("Location")
	}
	if got := login("/doc/notes/card"); got != "/doc/notes/card" {
		t.Errorf("после ключа увело на %q, а не обратно на экран", got)
	}
	for _, evil := range []string{"//evil.example", "https://evil.example", "/\\evil.example"} {
		if got := login(evil); got != "/" {
			t.Errorf("next=%q увёл на %q — открытый редирект", evil, got)
		}
	}
	// гость на экране приложения получает форму входа с обратным адресом
	rec := httptest.NewRecorder()
	handleRoot(rec, httptest.NewRequest(http.MethodGet, "/doc/notes/card", nil))
	if loc := rec.Header().Get("Location"); loc != "/?next=%2Fdoc%2Fnotes%2Fcard" {
		t.Errorf("гость отправлен на %q", loc)
	}
	// и замок несёт этот адрес в форму
	rec = httptest.NewRecorder()
	handleRoot(rec, httptest.NewRequest(http.MethodGet, "/?next=%2Fdoc%2Fnotes%2Fcard", nil))
	if !strings.Contains(rec.Body.String(), `value="/doc/notes/card"`) {
		t.Errorf("обратный адрес не попал в форму входа")
	}
	// неверный ключ не теряет обратный адрес
	rec = postForm(handleLogin, "/login", url.Values{"password": {"мимо"}, "next": {"/doc/notes/card"}}, false)
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "next=%2Fdoc%2Fnotes%2Fcard") {
		t.Errorf("после неверного ключа обратный адрес потерян: %q", loc)
	}
}

func TestAPIДашбордОдинОтветТолькоПоСессии(t *testing.T) {
	withAccess(t, "ключ-для-теста")
	get := func(path string, admin bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if admin {
			req = asAdmin(req)
		}
		rec := httptest.NewRecorder()
		handleBoard(rec, req)
		return rec
	}
	if rec := get("/api/board", false); rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "calendar") {
		t.Fatalf("гость получил дашборд: %d", rec.Code)
	}
	rec := get("/api/board?cal=2026-01&day=2026-01-01", true)
	var b map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("битый JSON: %v", err)
	}
	for _, k := range []string{"today", "calendar", "diary", "deadlines", "wheel"} {
		if _, ok := b[k]; !ok {
			t.Errorf("в ответе нет %q", k)
		}
	}
	// выбранный день календаря приходит со своим срезом: столпы и запись дневника за 1 января
	var cal struct {
		Month string `json:"month"`
		Day   struct {
			Date  string       `json:"date"`
			Vows  []boardPair  `json:"vows"`
			Diary []boardEntry `json:"diary"`
		} `json:"day"`
	}
	json.Unmarshal(b["calendar"], &cal)
	if cal.Month != "2026-01" || cal.Day.Date != "2026-01-01" || len(cal.Day.Vows) == 0 || len(cal.Day.Diary) == 0 {
		t.Errorf("срез дня не тот: %+v", cal)
	}
}

func TestAPIЭкраныКарточкиИПравки(t *testing.T) {
	withAccess(t, "ключ-для-теста")
	call := func(h http.HandlerFunc, method, path string, form url.Values, admin bool) *httptest.ResponseRecorder {
		var req *http.Request
		if form != nil {
			req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set(apiHeader, "1")
		if admin {
			req = asAdmin(req)
		}
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec
	}
	for _, c := range []struct {
		h    http.HandlerFunc
		path string
	}{{handleDocAPI, "/api/doc?id=notes/card"}, {handleListAPI, "/api/list?q=тело"}, {handleCardAPI, "/api/card?id=notes/card"}, {handleAccessAPI, "/api/access"}} {
		if rec := call(c.h, http.MethodGet, c.path, nil, false); rec.Code != http.StatusUnauthorized {
			t.Errorf("гость получил %s: %d", c.path, rec.Code)
		}
	}

	rec := call(handleDocAPI, http.MethodGet, "/api/doc?id=notes/card", nil, true)
	var doc docPage
	if json.Unmarshal(rec.Body.Bytes(), &doc); doc.Title != "Карточка" || !strings.Contains(doc.HTML, "тело") {
		t.Errorf("карточка: %d %s", rec.Code, rec.Body)
	}
	if rec := call(handleDocAPI, http.MethodGet, "/api/doc?id=нет/такой", nil, true); rec.Code != http.StatusNotFound {
		t.Errorf("несуществующая карточка: %d", rec.Code)
	}
	rec = call(handleListAPI, http.MethodGet, "/api/list?q="+url.QueryEscape("тело"), nil, true)
	if !strings.Contains(rec.Body.String(), `"kind":"search"`) || !strings.Contains(rec.Body.String(), "notes/card") {
		t.Errorf("поиск: %s", rec.Body)
	}

	// правка: свежая версия пишет, устаревшая — конфликт с версией на диске
	var raw cardRaw
	json.Unmarshal(call(handleCardAPI, http.MethodGet, "/api/card?id=notes/card", nil, true).Body.Bytes(), &raw)
	ok := call(handleCardAPI, http.MethodPost, "/api/card", url.Values{"id": {"notes/card"}, "content": {raw.Raw + "\nдобавлено\n"}, "ver": {raw.Ver}}, true)
	if ok.Code != http.StatusOK || !strings.Contains(ok.Body.String(), `"ok":true`) {
		t.Fatalf("правка не сохранилась: %d %s", ok.Code, ok.Body)
	}
	stale := call(handleCardAPI, http.MethodPost, "/api/card", url.Values{"id": {"notes/card"}, "content": {"затёр бы"}, "ver": {raw.Ver}}, true)
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "добавлено") {
		t.Errorf("устаревшая версия не поймана: %d %s", stale.Code, stale.Body)
	}
	// без X-Pillar-Api запись не принимается: форма с чужого сайта сюда не дойдёт
	req := asAdmin(httptest.NewRequest(http.MethodPost, "/api/card", strings.NewReader("id=notes/card&content=x")))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handleCardAPI(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("запись без заголовка приложения: %d", rec.Code)
	}
}

func TestВажныеЗаметкиЗакрепляютсяВКарточке(t *testing.T) {
	// правка frontmatter: строка добавляется и снимается, остальное не трогается
	raw := "---\ntype: note\nedges:\n  - id: a\n    pin: x\nsummary: \"s\"\n---\n# Т\npin: в теле не трогаем\n"
	on, err := setPin(raw, true)
	if err != nil || !strings.Contains(on, "summary: \"s\"\npin: true\n---") || !strings.Contains(on, "    pin: x") {
		t.Fatalf("закрепление: %v\n%s", err, on)
	}
	if again, _ := setPin(on, true); strings.Count(again, "pin: true") != 1 {
		t.Errorf("повтор задвоил строку:\n%s", again)
	}
	if off, _ := setPin(on, false); off != raw {
		t.Errorf("открепление не вернуло карточку как была:\n%s", off)
	}
	if _, err := setPin("# без frontmatter\n", true); err == nil {
		t.Error("карточку без frontmatter «закрепили»")
	}

	dir := withAccess(t, "ключ-для-теста")
	if rec := apiPost(handlePin, "/pin", url.Values{"id": {"notes/card"}, "on": {"1"}}, false); rec.Code == http.StatusOK {
		t.Fatalf("гость закрепил карточку: %d", rec.Code)
	}
	if res := apiResultOf(t, apiPost(handlePin, "/pin", url.Values{"id": {"notes/card"}, "on": {"1"}}, true)); !res.OK {
		t.Fatalf("не закрепилось: %+v", res)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "notes", "card.md"))
	if !strings.Contains(string(body), "pin: true") {
		t.Fatalf("в карточке нет pin:\n%s", body)
	}
	rec := httptest.NewRecorder()
	handleBoard(rec, asAdmin(httptest.NewRequest(http.MethodGet, "/api/board", nil)))
	var b struct {
		Pins []widgetPin `json:"pins"`
	}
	if json.Unmarshal(rec.Body.Bytes(), &b); len(b.Pins) != 1 || b.Pins[0].ID != "notes/card" {
		t.Errorf("на дашборде не та «Важное»: %+v", b.Pins)
	}
	rec = httptest.NewRecorder()
	handlePinsJSON(rec, httptest.NewRequest(http.MethodGet, "/pins.json", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("гость получил виджет «Важное»: %d", rec.Code)
	}
	if res := apiResultOf(t, apiPost(handlePin, "/pin", url.Values{"id": {"notes/card"}, "on": {"0"}}, true)); !res.OK {
		t.Fatalf("не откреплено: %+v", res)
	}
	if body, _ := os.ReadFile(filepath.Join(dir, "notes", "card.md")); strings.Contains(string(body), "pin:") {
		t.Errorf("строка pin осталась:\n%s", body)
	}
}

func TestЗахватПадаетВInbox(t *testing.T) {
	dir := withAccess(t, "ключ-для-теста")
	card := filepath.Join(dir, "inbox", "capture-ideas.md")
	os.MkdirAll(filepath.Dir(card), 0o755)
	os.WriteFile(card, []byte("---\ntype: area\nsummary: \"захват\"\ntags: [pkm]\nstatus: draft\n---\n# Захват\n\n## 💡 Новые идеи\n\n## 🔗 Ссылки для изучения\n\n---\n\n> подвал\n"), 0o644)
	rebuild()

	if rec := apiPost(handleCapture, "/capture", url.Values{"text": {"мысль"}}, false); rec.Code == http.StatusOK {
		t.Fatalf("гость записал в inbox: %d", rec.Code)
	}
	if res := apiResultOf(t, apiPost(handleCapture, "/capture", url.Values{"text": {"  купить  идею \n"}}, true)); !res.OK {
		t.Fatalf("мысль не записана: %+v", res)
	}
	if res := apiResultOf(t, apiPost(handleCapture, "/capture", url.Values{"text": {"https://example.org/x"}}, true)); !res.OK {
		t.Fatalf("ссылка не записана: %+v", res)
	}
	if res := apiResultOf(t, apiPost(handleCapture, "/capture", url.Values{"text": {"   "}}, true)); res.OK {
		t.Error("пустую мысль приняли")
	}
	raw, _ := os.ReadFile(card)
	s := string(raw)
	ideas, links, foot := strings.Index(s, "💡"), strings.Index(s, "🔗"), strings.Index(s, "> подвал")
	idea, link := strings.Index(s, "купить идею"), strings.Index(s, "https://example.org/x")
	if !(ideas < idea && idea < links && links < link && link < foot) {
		t.Fatalf("строки легли не в свои разделы:\n%s", s)
	}
}

func TestОкноДняКалендаря(t *testing.T) {
	withAccess(t, "ключ-для-теста")
	get := func(q string, admin bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/day.json"+q, nil)
		if admin {
			req = asAdmin(req)
		}
		rec := httptest.NewRecorder()
		handleDayJSON(rec, req)
		return rec
	}
	if rec := get("?date=2026-01-01", false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("гость получил день: %d", rec.Code)
	}
	if rec := get("?date=вчера", true); rec.Code != http.StatusBadRequest {
		t.Errorf("кривая дата: %d", rec.Code)
	}
	var d boardDay
	json.Unmarshal(get("?date=2026-01-01", true).Body.Bytes(), &d)
	if d.Date != "2026-01-01" || len(d.Vows) == 0 || len(d.Diary) == 0 {
		t.Errorf("срез дня не тот: %+v", d)
	}
}

func TestНоваяКарточкаИзПриложения(t *testing.T) {
	if s := slug("Сон и восстановление: 8 часов!"); s != "son-i-vosstanovlenie-8-chasov" {
		t.Errorf("slug: %q", s)
	}
	dir := withAccess(t, "ключ-для-теста")
	post := func(f url.Values, admin bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/new", strings.NewReader(f.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set(apiHeader, "1")
		if admin {
			req = asAdmin(req)
		}
		rec := httptest.NewRecorder()
		handleNewAPI(rec, req)
		return rec
	}
	ok := url.Values{"title": {"Сон и восстановление"}, "folder": {"notes"}, "type": {"note"},
		"summary": {`Режим сна: "до полуночи", 7–8 часов`}, "tag": {"здоровье"}, "tags": {"сон, Режим дня"},
		"body": {"Первая мысль."}}

	if rec := post(ok, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("гость создал карточку: %d", rec.Code)
	}
	for name, bad := range map[string]url.Values{
		"без summary":        {"title": {"x"}, "folder": {"notes"}, "type": {"note"}, "tag": {"pkm"}},
		"тег не из дерева":   {"title": {"x"}, "folder": {"notes"}, "type": {"note"}, "summary": {"s"}, "tag": {"сон"}},
		"два тега из дерева": {"title": {"x"}, "folder": {"notes"}, "type": {"note"}, "summary": {"s"}, "tag": {"pkm"}, "tags": {"цели"}},
		"раздел вне":         {"title": {"x"}, "folder": {"../server"}, "type": {"note"}, "summary": {"s"}, "tag": {"pkm"}},
		"служебный раздел":   {"title": {"x"}, "folder": {"_sys"}, "type": {"note"}, "summary": {"s"}, "tag": {"pkm"}},
		"кривой тег":         {"title": {"x"}, "folder": {"notes"}, "type": {"note"}, "summary": {"s"}, "tag": {"pkm"}, "tags": {"a/b"}},
	} {
		if rec := post(bad, true); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: принято (%d) %s", name, rec.Code, rec.Body)
		}
	}

	rec := post(ok, true)
	var res struct {
		OK bool
		ID string
	}
	json.Unmarshal(rec.Body.Bytes(), &res)
	if !res.OK || res.ID != "notes/son-i-vosstanovlenie" {
		t.Fatalf("не создана: %d %s", rec.Code, rec.Body)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "notes", "son-i-vosstanovlenie.md"))
	want := "---\ntype: note\nsummary: \"Режим сна: «до полуночи», 7–8 часов\"\ntags: [здоровье, сон, режим-дня]\nstatus: draft\n"
	if !strings.HasPrefix(string(raw), want) || !strings.Contains(string(raw), "# Сон и восстановление\n\nПервая мысль.\n") {
		t.Fatalf("карточка не та:\n%s", raw)
	}
	if d := currentVault().Docs[res.ID]; d == nil || d.Summary != "Режим сна: «до полуночи», 7–8 часов" {
		t.Errorf("модель не пересобрана или summary разобран не так: %+v", d)
	}
	// то же имя — второй файл с номером, первый не затёрт
	json.Unmarshal(post(ok, true).Body.Bytes(), &res)
	if res.ID != "notes/son-i-vosstanovlenie-2" {
		t.Errorf("второе имя: %q", res.ID)
	}
	if ok, out := runLint(t, dir); !ok && strings.Contains(out, "son-i-vosstanovlenie") {
		t.Errorf("линтер против новой карточки:\n%s", out)
	}

	os.MkdirAll(filepath.Join(dir, "goals"), 0o755)
	goal := url.Values{"title": {"Подушка"}, "folder": {"goals"}, "type": {"goal"}, "summary": {"три оклада"}, "tag": {"цели"}, "deadline": {"2026-12-31"}}
	json.Unmarshal(post(goal, true).Body.Bytes(), &res)
	g, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(res.ID)+".md"))
	if !strings.Contains(string(g), "deadline: 2026-12-31\n") || !strings.Contains(string(g), goalKRHeading) {
		t.Errorf("цель без срока или заготовки результатов:\n%s", g)
	}
}

func TestВходящиеРазбираютсяБезАгента(t *testing.T) {
	dir := withAccess(t, "ключ-для-теста")
	write := func(rel, body string) {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
		os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644)
	}
	write("inbox/capture-ideas.md", "---\ntype: area\nsummary: \"захват\"\ntags: [pkm]\nstatus: draft\n---\n# Захват\n\n## 💡 Новые идеи\n\n"+
		"- 2026-10-21 10:00 позвонить в банк\n- 2026-10-21 11:00 идея про сад\n\n## 🔗 Ссылки для изучения\n\n- 2026-10-21 12:00 https://example.org\n\n---\n\n> - не мысль, подвал\n")
	write("tasks/backlog.md", "---\ntype: todo\nsummary: \"бэклог\"\ntags: [дела]\nstatus: draft\n---\n# Бэклог\n\n## Средний приоритет 🟡\n\n- [ ] старое дело\n")
	rebuild()

	items := inboxOf(currentVault()).Items
	if len(items) != 3 || items[0].Clean != "позвонить в банк" || items[0].When != "2026-10-21 10:00" || !items[2].Link {
		t.Fatalf("мысли: %+v", items)
	}
	if postForm(handleInbox, "/inbox", url.Values{"do": {"task"}, "text": {items[0].Text}}, false); len(inboxOf(currentVault()).Items) != 3 {
		t.Fatal("гость разобрал входящие")
	}
	if res := apiResultOf(t, apiPost(handleInbox, "/inbox", url.Values{"do": {"task"}, "text": {items[0].Text}}, true)); !res.OK {
		t.Fatalf("в дела: %+v", res)
	}
	backlog, _ := os.ReadFile(filepath.Join(dir, "tasks", "backlog.md"))
	if !strings.Contains(string(backlog), "- [ ] позвонить в банк") {
		t.Errorf("дело не в бэклоге:\n%s", backlog)
	}
	if res := apiResultOf(t, apiPost(handleInbox, "/inbox", url.Values{"do": {"remove"}, "text": {items[1].Text}}, true)); !res.OK {
		t.Fatalf("удалить: %+v", res)
	}
	if res := apiResultOf(t, apiPost(handleInbox, "/inbox", url.Values{"do": {"remove"}, "text": {"выдуманная"}}, true)); res.OK {
		t.Error("удалили то, чего нет")
	}
	left := inboxOf(currentVault()).Items
	if len(left) != 1 || !left[0].Link {
		t.Errorf("осталось: %+v", left)
	}
	card, _ := os.ReadFile(filepath.Join(dir, "inbox", "capture-ideas.md"))
	if !strings.Contains(string(card), "> - не мысль, подвал") {
		t.Errorf("подвал карточки задет:\n%s", card)
	}
}

func TestЗдоровьеХранилищаВПриложении(t *testing.T) {
	dir := withAccess(t, "ключ-для-теста")
	get := func(admin bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		if admin {
			req = asAdmin(req)
		}
		rec := httptest.NewRecorder()
		handleHealthAPI(rec, req)
		return rec
	}
	if rec := get(false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("гость получил отчёт: %d", rec.Code)
	}
	os.WriteFile(filepath.Join(dir, "notes", "broken.md"), []byte("---\ntype: note\ntags: [pkm]\nstatus: draft\n---\n# Сломанная\n\nсм. [[нет-такой]]\n"), 0o644)
	rebuild()
	var rep healthReport
	json.Unmarshal(get(true).Body.Bytes(), &rep)
	whats := []string{}
	for _, e := range rep.Errors {
		if e.ID == "notes/broken" && e.Title == "Сломанная" {
			whats = append(whats, e.What)
		}
	}
	if len(whats) != 2 {
		t.Fatalf("ошибки сломанной карточки: %+v", rep.Errors)
	}
	// починил — после пересборки отчёт свежий, а не из кэша
	os.WriteFile(filepath.Join(dir, "notes", "broken.md"), []byte("---\ntype: note\nsummary: \"ок\"\ntags: [pkm]\nstatus: draft\n---\n# Сломанная\n"), 0o644)
	time.Sleep(10 * time.Millisecond)
	rebuild()
	json.Unmarshal(get(true).Body.Bytes(), &rep)
	for _, e := range rep.Errors {
		if e.ID == "notes/broken" {
			t.Errorf("после правки ошибка осталась: %+v", e)
		}
	}
}

func TestЦельФормамиИзВиджета(t *testing.T) {
	dir := withAccess(t, "ключ-для-теста")
	os.MkdirAll(filepath.Join(dir, "goals"), 0o755)
	os.WriteFile(filepath.Join(dir, "goals", "g.md"), []byte("---\ntype: goal\nsummary: \"s\"\ntags: [цели]\nstatus: draft\ncreated: 2026-01-01\n---\n# Подушка\n\n## Ключевые результаты\n\n| Дата | "+goalPlaceholder+" |\n|---|---|\n| Цель | |\n"), 0o644)
	rebuild()
	do := func(f url.Values) apiResult {
		f.Set("id", "goals/g")
		return apiResultOf(t, apiPost(handleGoal, "/goal", f, true))
	}
	if r := do(url.Values{"do": {"kr"}, "name": {"Подушка"}, "unit": {"₽ тыс"}, "target": {"300"}}); !r.OK {
		t.Fatalf("результат: %+v", r)
	}
	if r := do(url.Values{"do": {"kr"}, "name": {"Книги"}, "unit": {"шт"}, "target": {"12"}}); !r.OK {
		t.Fatalf("второй результат: %+v", r)
	}
	if r := do(url.Values{"do": {"value"}, "date": {"2026-02-01"}, "v0": {"40"}, "v1": {"1"}}); !r.OK {
		t.Fatalf("показание: %+v", r)
	}
	if r := do(url.Values{"do": {"value"}, "date": {"2026-02-01"}, "v1": {"2,5"}}); !r.OK {
		t.Fatalf("то же число дописано: %+v", r)
	}
	if r := do(url.Values{"do": {"value"}, "date": {"2026-02-01"}, "v0": {"много"}}); r.OK {
		t.Error("не число приняли")
	}
	if r := do(url.Values{"do": {"deadline"}, "deadline": {"2026-12-31"}}); !r.OK {
		t.Fatalf("срок: %+v", r)
	}
	g := goalOf(currentVault(), currentVault().Docs["goals/g"], "2026-03-01")
	if g == nil || g.Deadline != "2026-12-31" || len(g.KRs) != 2 || g.KRs[0].Current != 40 || g.KRs[1].Current != 2.5 || g.KRs[1].Col != 1 {
		t.Fatalf("цель после форм: %+v", g)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "goals", "g.md"))
	if !strings.Contains(string(raw), "| Дата | Подушка, ₽ тыс | Книги, шт |\n|---|---|---|\n| Цель | 300 | 12 |\n| 2026-02-01 | 40 | 2.5 |") {
		t.Errorf("таблица в карточке:\n%s", raw)
	}
}

func TestАрхивУбираетИзПоискаИВажного(t *testing.T) {
	dir := withAccess(t, "ключ-для-теста")
	apiResultOf(t, apiPost(handlePin, "/pin", url.Values{"id": {"notes/card"}, "on": {"1"}}, true))
	if len(search(currentVault(), "тело")) == 0 || len(pinned(currentVault())) != 1 {
		t.Fatal("до архива карточка не находится или не закреплена")
	}
	if postForm(handleArchive, "/archive", url.Values{"id": {"notes/card"}, "on": {"1"}}, false); currentVault().Docs["notes/card"].Status == "archived" {
		t.Fatal("гость отправил в архив")
	}
	if r := apiResultOf(t, apiPost(handleArchive, "/archive", url.Values{"id": {"notes/card"}, "on": {"1"}}, true)); !r.OK {
		t.Fatalf("в архив: %+v", r)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "notes", "card.md"))
	if !strings.Contains(string(raw), "status: archived") || strings.Contains(string(raw), "pin:") {
		t.Fatalf("карточка после архива:\n%s", raw)
	}
	for _, h := range search(currentVault(), "тело") {
		if h.Doc.ID == "notes/card" {
			t.Error("архивная карточка в поиске")
		}
	}
	if len(pinned(currentVault())) != 0 {
		t.Error("архивная карточка в «Важном»")
	}
	apiResultOf(t, apiPost(handleArchive, "/archive", url.Values{"id": {"notes/card"}, "on": {"0"}}, true))
	if currentVault().Docs["notes/card"].Status != "draft" || len(search(currentVault(), "тело")) == 0 {
		t.Error("не вернулась из архива")
	}
}
