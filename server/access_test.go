// Границу приватности держат не комментарии, а эти тесты: куда можно писать,
// что видит гость, чем подписан билет и что формы дашборда правят карточку.
package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// vaultFixture — минимальное хранилище на диске: карточки, замок, сборка
// приложения и заведомо личный файл рядом с интерфейсом.
func vaultFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("SOUL.md", "# устав\n")
	write("notes/card.md", "---\ntype: note\nsummary: \"карточка\"\ntags: [pkm]\nstatus: draft\n---\n# Карточка\nтело\n")
	write("notes/diary.md", "---\ntype: note\nsummary: \"дневник\"\ntags: [pkm]\nstatus: draft\n---\n# Дневник\n\n## 2026-01-01 10:00\n\nстарая запись\n")
	write("pillars/daily-vows.md", "---\ntype: pillar\nsummary: \"столпы\"\ntags: [цели]\nstatus: complete\n---\n"+
		"## Столпы\n\n- **Сон** — до полуночи\n- **Вода** — два литра\n\n"+
		"## Журнал\n\n| Дата | Сон | Вода |\n|---|---|---|\n| 2026-01-01 | ✓ | ✓ |\n")
	write("viewer/gate.css", "/* замок */")
	write("viewer/gate.js", "// дождь")
	write("viewer/x/index.html", "<!doctype html><div id=root></div>")
	write("viewer/x/assets/app-1234.js", "console.log('приложение')")
	write("viewer/fonts.css", "/* шрифты */")
	write("viewer/fonts/jetbrains-mono-400-cyrillic.woff2", "woff2")
	write("viewer/gallery/config.json", `{"личное":true}`)

	// замок берём настоящий: тест должен ловить и ошибки в нём
	raw, err := os.ReadFile(filepath.Join("..", "viewer", "templates", "lock.html"))
	if err != nil {
		t.Fatalf("шаблон замка не найден: %v", err)
	}
	write("viewer/templates/lock.html", string(raw))
	return dir
}

// withVault поднимает пакетные переменные и шаблоны на время теста.
func withVault(t *testing.T, dir, hash string) {
	t.Helper()
	oldRoot, oldHash := root, passHash()
	root = dir
	setPassHash(hash)
	t.Cleanup(func() { root = oldRoot; setPassHash(oldHash) })
	if err := rebuild(); err != nil {
		t.Fatalf("сборка модели: %v", err)
	}
	if err := loadTemplates(); err != nil {
		t.Fatalf("шаблоны: %v", err)
	}
}

// asAdmin — запрос с настоящим билетом сессии.
func asAdmin(r *http.Request) *http.Request {
	token, _ := newSession()
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	return r
}

func TestCardPathДержитсяВнутриХранилища(t *testing.T) {
	dir := vaultFixture(t)
	withVault(t, dir, "")

	for _, rel := range []string{"notes/card.md", "/notes/card.md", "inbox/new.md", "facts/a.MD"} {
		if _, err := cardPath(rel); err != nil {
			t.Errorf("cardPath(%q) отказал, а должен пустить: %v", rel, err)
		}
	}
	bad := []string{
		"",                      // пусто
		"notes/card.txt",        // не карточка
		"viewer/app.md",         // интерфейс
		"server/main.md",        // код
		"deploy/deploy.md",      // деплой
		".git/config.md",        // репозиторий
		"node_modules/pkg/x.md", // зависимости
	}
	for _, rel := range bad {
		if full, err := cardPath(rel); err == nil {
			t.Errorf("cardPath(%q) пустил, а не должен: %s", rel, full)
		}
	}
	// Путь вверх по дереву не отвергается, а подрезается внутрь хранилища.
	// Создать файл этим нельзя: saveCard пишет только в существующие карточки.
	for _, rel := range []string{"../../etc/passwd.md", "notes/../../escape.md", "/etc/passwd.md"} {
		full, err := cardPath(rel)
		if err != nil {
			continue
		}
		if !strings.HasPrefix(full, dir+string(os.PathSeparator)) {
			t.Errorf("cardPath(%q) вывел за пределы хранилища: %s", rel, full)
		}
	}
}

func TestГостьВидитТолькоФормуВхода(t *testing.T) {
	dir := vaultFixture(t)
	hash, _ := hashPassword("правильный-ключ")
	withVault(t, dir, hash)
	get := func(path string, admin bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if admin {
			req = asAdmin(req)
		}
		rec := httptest.NewRecorder()
		handleRoot(rec, req)
		return rec
	}

	// главная без сессии — замок, и ни одной карточки, ни файла приложения
	rec := get("/", false)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "gate-input") {
		t.Fatalf("гостю не показали форму входа: код %d", rec.Code)
	}
	for _, leak := range []string{"Карточка", "notes/card", "Сон", "дневник", "id=root"} {
		if strings.Contains(body, leak) {
			t.Errorf("в форму входа утекло «%s»", leak)
		}
	}

	// экраны приложения гостю — редирект на замок с обратным адресом
	for _, p := range []string{"/doc/notes/card", "/folder/notes", "/tag/pkm", "/search?q=карточка", "/edit/notes/card", "/access"} {
		rec := get(p, false)
		if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/?next=") {
			t.Errorf("%s без сессии: код %d → %q, ожидался замок", p, rec.Code, rec.Header().Get("Location"))
		}
		if strings.Contains(rec.Body.String(), "notes/card") || strings.Contains(rec.Body.String(), "root") {
			t.Errorf("%s без сессии отдал содержимое", p)
		}
	}
	if rec := get("/assets/app-1234.js", false); rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "приложение") {
		t.Errorf("гость получил сборку приложения: %d", rec.Code)
	}

	// статика: только то, чем рисуется замок
	for _, p := range []string{"/gate.css", "/gate.js", "/fonts.css", "/fonts/jetbrains-mono-400-cyrillic.woff2"} {
		if rec := get(p, false); rec.Code != http.StatusOK {
			t.Errorf("гостю не отдали %s: код %d", p, rec.Code)
		}
	}
	for _, p := range []string{"/gallery/config.json", "/templates/lock.html", "/x/index.html", "/../SOUL.md"} {
		if rec := get(p, false); rec.Code == http.StatusOK {
			t.Errorf("гостю утекло %s", p)
		}
	}
}

func TestПоСессииОтдаётсяПриложение(t *testing.T) {
	dir := vaultFixture(t)
	hash, _ := hashPassword("правильный-ключ")
	withVault(t, dir, hash)
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handleRoot(rec, asAdmin(httptest.NewRequest(http.MethodGet, path, nil)))
		return rec
	}

	// любой экран приложения — его index.html, маршрут разбирает уже браузер
	for _, p := range []string{"/", "/doc/notes/card", "/search?q=x", "/access"} {
		if rec := get(p); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "id=root") {
			t.Errorf("%s по сессии: код %d", p, rec.Code)
		}
	}
	// старые адреса /x/… ведут туда же, без префикса
	if rec := get("/x/doc/notes/card?a=1"); rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/doc/notes/card?a=1" {
		t.Errorf("/x/ не перенаправлен: %d %q", rec.Code, rec.Header().Get("Location"))
	}

	// карточка — данными: заголовок не дублируется разметкой тела
	req := asAdmin(httptest.NewRequest(http.MethodGet, "/api/doc?id=notes/card", nil))
	rec := httptest.NewRecorder()
	handleDocAPI(rec, req)
	var doc docPage
	if json.Unmarshal(rec.Body.Bytes(), &doc); !strings.Contains(doc.HTML, "тело") {
		t.Fatalf("карточка по сессии: код %d", rec.Code)
	}
	if strings.Contains(doc.HTML, "<h1") {
		t.Errorf("заголовок карточки повторён в теле: %s", doc.HTML)
	}
}

func TestОтметкаСтолпаПишетсяВКарточку(t *testing.T) {
	dir := vaultFixture(t)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)

	form := url.Values{"name": {"Сон"}, "mark": {vowHeld}}
	req := asAdmin(httptest.NewRequest(http.MethodPost, "/vow", strings.NewReader(form.Encode())))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handleVow(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("отметка вернула %d, ожидался редирект", rec.Code)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "pillars", "daily-vows.md"))
	today := ymd(time.Now())
	if !strings.Contains(string(raw), "| "+today+" | ✓ |") {
		t.Fatalf("строка за сегодня не появилась в журнале:\n%s", raw)
	}

	// повторное нажатие снимает отметку
	rec = httptest.NewRecorder()
	req = asAdmin(httptest.NewRequest(http.MethodPost, "/vow", strings.NewReader(form.Encode())))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handleVow(rec, req)
	raw, _ = os.ReadFile(filepath.Join(dir, "pillars", "daily-vows.md"))
	if strings.Contains(string(raw), "| "+today+" | ✓ |") {
		t.Fatalf("повторное нажатие не сняло отметку:\n%s", raw)
	}

	// без сессии журнал не правится
	before, _ := os.ReadFile(filepath.Join(dir, "pillars", "daily-vows.md"))
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/vow", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handleVow(rec, req)
	after, _ := os.ReadFile(filepath.Join(dir, "pillars", "daily-vows.md"))
	if string(before) != string(after) {
		t.Fatal("журнал столпов изменился без сессии")
	}
}

func TestЗаписьВДневникДописываетсяВКонец(t *testing.T) {
	dir := vaultFixture(t)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)

	form := url.Values{"text": {"Проверка записи."}}
	req := asAdmin(httptest.NewRequest(http.MethodPost, "/diary", strings.NewReader(form.Encode())))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handleDiary(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("запись вернула %d", rec.Code)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "notes", "diary.md"))
	if !strings.Contains(string(raw), "Проверка записи.") {
		t.Fatalf("записи нет в карточке:\n%s", raw)
	}
	if !strings.Contains(string(raw), "старая запись") {
		t.Fatal("прежние записи затёрты")
	}
	if !strings.Contains(string(raw), "## "+ymd(time.Now())) {
		t.Fatal("нет заголовка с датой")
	}
	// модель пересобрана — страница увидит новую запись сразу
	if v := currentVault(); v.Diary == nil || v.Diary.Text != "Проверка записи." {
		t.Fatalf("модель не пересобрана после записи: %+v", v.Diary)
	}
}

func TestПравкаКарточкиТолькоПоСессии(t *testing.T) {
	dir := vaultFixture(t)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)

	save := func(admin bool) *httptest.ResponseRecorder {
		form := url.Values{"id": {"notes/card"}, "content": {"# Взломано\n"}}
		req := httptest.NewRequest(http.MethodPost, "/api/card", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set(apiHeader, "1")
		if admin {
			req = asAdmin(req)
		}
		rec := httptest.NewRecorder()
		handleCardAPI(rec, req)
		return rec
	}
	save(false)
	raw, _ := os.ReadFile(filepath.Join(dir, "notes", "card.md"))
	if strings.Contains(string(raw), "Взломано") {
		t.Fatal("карточку переписали без сессии")
	}

	if rec := save(true); rec.Code != http.StatusOK {
		t.Fatalf("правка по сессии вернула %d: %s", rec.Code, rec.Body)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, "notes", "card.md"))
	if !strings.Contains(string(raw), "Взломано") {
		t.Fatal("правка по сессии не сохранилась")
	}
}

func TestБилетСессии(t *testing.T) {
	hash, _ := hashPassword("ключ")
	oldHash := passHash()
	setPassHash(hash)
	t.Cleanup(func() { setPassHash(oldHash) })

	token, expires := newSession()
	if !validSession(token) {
		t.Fatal("свежий билет не принят")
	}
	if d := time.Until(expires); d < sessionTTL-time.Minute || d > sessionTTL+time.Minute {
		t.Errorf("срок билета %v, ожидался около %v", d, sessionTTL)
	}
	bad := map[string]string{
		"мусор":          "не-билет",
		"без подписи":    strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
		"чужая подпись":  strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + ".AAAAAAAAAAAAAAAAAAAAAA",
		"просроченный":   signSession(time.Now().Add(-time.Minute).Unix()),
		"срок переписан": strconv.FormatInt(time.Now().Add(100*time.Hour).Unix(), 10) + token[strings.IndexByte(token, '.'):],
		"пустая строка":  "",
	}
	for name, tok := range bad {
		if validSession(tok) {
			t.Errorf("принят негодный билет (%s): %q", name, tok)
		}
	}
	// смена пароля рвёт все сессии — на этом держится deploy/passwd.sh
	newHash, _ := hashPassword("другой-ключ")
	setPassHash(newHash)
	if validSession(token) {
		t.Fatal("билет пережил смену пароля")
	}
	setPassHash("")
	if validSession(token) {
		t.Fatal("билет принят при выключенном замке")
	}
}

func TestПарольПроверяетсяХэшем(t *testing.T) {
	hash, err := hashPassword("длинный-ключ-хозяина")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "pbkdf2-sha256$600000$") {
		t.Fatalf("неожиданный формат хэша: %s", hash)
	}
	if ok, err := verifyPassword("длинный-ключ-хозяина", hash); err != nil || !ok {
		t.Fatalf("верный пароль не подошёл: ok=%v err=%v", ok, err)
	}
	if ok, _ := verifyPassword("длинный-ключ-хозяин", hash); ok {
		t.Fatal("подошёл неверный пароль")
	}
	if other, _ := hashPassword("длинный-ключ-хозяина"); other == hash {
		t.Fatal("соль не случайна: одинаковые хэши")
	}
	for _, broken := range []string{"", "пусто", "pbkdf2-sha256$нечисло$c2FsdA$aGFzaA", "sha1$1$a$b"} {
		if _, err := verifyPassword("x", broken); err == nil {
			t.Errorf("битый хэш %q принят за настоящий", broken)
		}
	}
}

func TestСчётчикПопытокНеРастётВечно(t *testing.T) {
	a := newAttemptStore()
	for i := 0; i < maxTrackedIPs*2; i++ {
		ip := "10.0." + strconv.Itoa(i/256) + "." + strconv.Itoa(i%256)
		a.allow(ip)
		a.fail(ip)
	}
	a.mu.Lock()
	for ip := range a.hits {
		a.hits[ip] = []time.Time{time.Now().Add(-attemptWindow - time.Minute)}
	}
	a.mu.Unlock()
	a.fail("10.9.9.9")
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.hits) > maxTrackedIPs {
		t.Fatalf("карта попыток разрослась: %d записей", len(a.hits))
	}
}

func TestПорогПопытокЗакрываетВход(t *testing.T) {
	a := newAttemptStore()
	const ip = "203.0.113.7"
	for i := 0; i < maxAttempts; i++ {
		if !a.allow(ip) {
			t.Fatalf("вход закрылся на попытке %d из %d", i+1, maxAttempts)
		}
		a.fail(ip)
	}
	if a.allow(ip) {
		t.Fatal("после лимита попыток адрес всё ещё пускают")
	}
	a.reset(ip)
	if !a.allow(ip) {
		t.Fatal("после удачного входа адрес остался заблокирован")
	}
}

func TestДелоПереноситсяНаСегодня(t *testing.T) {
	dir := vaultFixture(t)
	day := "---\ntype: todo\nsummary: \"день\"\ntags: [дела]\nstatus: draft\n---\n# День\n\n## Текущий день\n\n_Пусто. Назначь дела на сегодня._\n\n---\n\n### Формат записи\n"
	backlog := "---\ntype: todo\nsummary: \"бэклог\"\ntags: [дела]\nstatus: draft\n---\n# Бэклог\n\n## Высокий приоритет\n\n- [ ] Позвонить врачу\n- [x] Закрытое дело\n"
	for rel, body := range map[string]string{"tasks/day.md": day, "tasks/backlog.md": backlog} {
		if err := os.MkdirAll(filepath.Join(dir, "tasks"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)

	post := func(text string) {
		form := url.Values{"text": {text}}
		req := asAdmin(httptest.NewRequest(http.MethodPost, "/today", strings.NewReader(form.Encode())))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		handleToday(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("перенос вернул %d", rec.Code)
		}
	}
	read := func() string {
		raw, _ := os.ReadFile(filepath.Join(dir, "tasks", "day.md"))
		return string(raw)
	}

	// закрытое и выдуманное дело в день не попадают
	post("Закрытое дело")
	post("Выдуманное\n## заголовок")
	if strings.Contains(read(), "- [ ]") {
		t.Fatalf("в день попало то, чего нет в открытых делах:\n%s", read())
	}

	post("Позвонить врачу")
	got := read()
	if !strings.Contains(got, "## Текущий день\n\n- [ ] Позвонить врачу\n\n---") {
		t.Fatalf("дело встало не туда:\n%s", got)
	}
	if strings.Contains(got, "_Пусто") {
		t.Fatal("заглушка осталась")
	}
	if l := todayList(currentVault()); len(l) != 1 || l[0].Text != "Позвонить врачу" {
		t.Fatalf("модель не видит дело на сегодня: %+v", l)
	}
	// уже стоящее на сегодня второй раз не выбирается
	post("Позвонить врачу")
	if strings.Count(read(), "Позвонить врачу") != 1 {
		t.Fatalf("дело задвоилось:\n%s", read())
	}
	// исходная строка в бэклоге не тронута
	if raw, _ := os.ReadFile(filepath.Join(dir, "tasks", "backlog.md")); !strings.Contains(string(raw), "- [ ] Позвонить врачу") {
		t.Fatal("дело пропало из бэклога")
	}
	// клик по делу дня ведёт в его карточку, а не в сам день
	if home := taskHome(currentVault(), todayList(currentVault())[0]); home != "tasks/backlog" {
		t.Fatalf("дело дня ведёт в %q", home)
	}

	// убрать из дня: строка уходит, в бэклоге дело открыто
	act := func(do, text string) {
		form := url.Values{"do": {do}, "text": {text}}
		req := asAdmin(httptest.NewRequest(http.MethodPost, "/today", strings.NewReader(form.Encode())))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		handleToday(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%s вернул %d", do, rec.Code)
		}
	}
	act("remove", "Позвонить врачу")
	if strings.Contains(read(), "Позвонить врачу") || !strings.Contains(read(), "_Пусто") {
		t.Fatalf("дело не убрано из дня:\n%s", read())
	}
	if len(todayList(currentVault())) != 0 {
		t.Fatal("модель всё ещё видит дело на сегодня")
	}

	// выполнено: закрывается в бэклоге, из дня уходит
	post("Позвонить врачу")
	act("done", "Позвонить врачу")
	backlogNow, _ := os.ReadFile(filepath.Join(dir, "tasks", "backlog.md"))
	if !strings.Contains(string(backlogNow), "- [x] Позвонить врачу") {
		t.Fatalf("дело не закрыто в бэклоге:\n%s", backlogNow)
	}
	if strings.Contains(read(), "Позвонить врачу") {
		t.Fatalf("закрытое дело осталось в дне:\n%s", read())
	}

	// дело, заведённое прямо в дне, закрывается в самом дне
	if err := os.WriteFile(filepath.Join(dir, "tasks", "day.md"), []byte(strings.Replace(day, "_Пусто. Назначь дела на сегодня._", "- [ ] Своё дело", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rebuild(); err != nil {
		t.Fatal(err)
	}
	act("done", "Своё дело")
	if !strings.Contains(read(), "- [x] Своё дело") {
		t.Fatalf("дело дня не закрыто:\n%s", read())
	}
	// чужой текст не закрывает и не убирает ничего
	act("done", "Выдуманное")
	act("remove", "Выдуманное")

	// своё дело из поля: переносы схлопываются в одну строку — заголовок
	// или таблицу в карточку не протащить; пустое и повторное не пишутся
	act("new", "  Купить\n## не заголовок  ")
	act("new", "   ")
	act("new", "Купить ## не заголовок")
	got = read()
	if strings.Count(got, "- [ ] Купить ## не заголовок") != 1 || strings.Contains(got, "\n## не заголовок") {
		t.Fatalf("своё дело записано не так:\n%s", got)
	}
	if l := todayList(currentVault()); len(l) != 1 || l[0].Text != "Купить ## не заголовок" || taskHome(currentVault(), l[0]) != "tasks/day" {
		t.Fatalf("своё дело не на сегодня или ведёт не в день: %+v", l)
	}
}

func TestАдресДляСчётчикаПопытокНеПодделать(t *testing.T) {
	// снаружи: заголовки пишет кто угодно — верим только адресу соединения
	r := httptest.NewRequest(http.MethodPost, "/login", nil)
	r.RemoteAddr = "203.0.113.7:5555"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	r.Header.Set("X-Real-IP", "5.6.7.8")
	if ip := clientIP(r); ip != "203.0.113.7" {
		t.Fatalf("внешний запрос подменил адрес: %s", ip)
	}
	// через свой nginx на loopback: X-Real-IP, а не клиентский X-Forwarded-For
	r.RemoteAddr = "127.0.0.1:40000"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 198.51.100.9")
	r.Header.Set("X-Real-IP", "198.51.100.9")
	if ip := clientIP(r); ip != "198.51.100.9" {
		t.Fatalf("за nginx взят не тот адрес: %s", ip)
	}
}

func TestУчётДелЗакрываетИОткрывает(t *testing.T) {
	dir := vaultFixture(t)
	writeCard := func(rel, body string) {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	head := "---\ntype: todo\nsummary: \"x\"\ntags: [дела]\nstatus: draft\n---\n"
	writeCard("tasks/backlog.md", head+"# Бэклог\n\n- [ ] Позвонить\n- [x] Сделанное\n")
	writeCard("tasks/day.md", head+"# День\n\n## Текущий день\n\n- [ ] Позвонить\n")
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)

	act := func(do, src, text string) {
		form := url.Values{"do": {do}, "src": {src}, "text": {text}}
		req := asAdmin(httptest.NewRequest(http.MethodPost, "/task", strings.NewReader(form.Encode())))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		handleTask(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%s вернул %d", do, rec.Code)
		}
	}
	read := func(rel string) string {
		raw, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		return string(raw)
	}

	act("done", "tasks/backlog", "Позвонить")
	if !strings.Contains(read("tasks/backlog.md"), "- [x] Позвонить") {
		t.Fatalf("не закрыто:\n%s", read("tasks/backlog.md"))
	}
	if strings.Contains(read("tasks/day.md"), "Позвонить") {
		t.Fatal("закрытое из учёта осталось в «Сегодня»")
	}
	act("open", "tasks/backlog", "Сделанное")
	if !strings.Contains(read("tasks/backlog.md"), "- [ ] Сделанное") {
		t.Fatalf("не открыто снова:\n%s", read("tasks/backlog.md"))
	}
	// чужое и несуществующее не трогается: день закрыть отсюда нельзя
	before := read("tasks/backlog.md")
	act("done", "tasks/backlog", "Выдуманное")
	act("done", "notes/card", "Позвонить")
	act("open", "tasks/backlog", "Сделанное") // уже открыто
	if read("tasks/backlog.md") != before {
		t.Fatal("правка по несуществующему делу")
	}
}

func TestПравкаСВебаКоммитится(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("нет git")
	}
	dir := vaultFixture(t)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "старт"}} {
		if out, err := git(args...); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	autoCommit = true
	t.Cleanup(func() { autoCommit = false })

	if err := saveCard("notes/card.md", "новое тело\n", "правка карточки"); err != nil {
		t.Fatal(err)
	}
	if out, _ := git("log", "-1", "--format=%s|%an"); out != "веб: правка карточки|Pillar Vault" {
		t.Fatalf("коммита нет или он не тот: %q", out)
	}
	if out, _ := git("status", "--porcelain"); out != "" {
		t.Fatalf("после коммита остались правки: %q", out)
	}
}

func TestФотоВитриныСВеба(t *testing.T) {
	dir := vaultFixture(t)
	gdir := filepath.Join(dir, "viewer", "gallery")
	cfgJSON := `{"title":"Витрина","slides":[{"src":"gallery/mock-1.svg","caption":"Мок","sub":"м"}]}`
	if err := os.WriteFile(filepath.Join(gdir, "config.json"), []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gdir, "mock-1.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)

	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89")
	post := func(admin bool, fields map[string]string, photo []byte) string {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for k, v := range fields {
			mw.WriteField(k, v)
		}
		if photo != nil {
			fw, _ := mw.CreateFormFile("photo", "x.bin")
			fw.Write(photo)
		}
		mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/gallery", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		if admin {
			req = asAdmin(req)
		}
		rec := httptest.NewRecorder()
		handleGallery(rec, req)
		return rec.Header().Get("Location")
	}
	cfg := func() *galleryConfig { c, _ := readGallery(); return c }
	photos := func() []string { m, _ := filepath.Glob(filepath.Join(gdir, "photo-*")); return m }

	// гостю — ничего
	post(false, map[string]string{"slot": "new", "caption": "чужое"}, png)
	if len(cfg().Slides) != 1 || len(photos()) != 0 {
		t.Fatal("гость поменял витрину")
	}
	// не картинка не принимается
	if loc := post(true, map[string]string{"slot": "new"}, []byte("<?php echo 1; ?>")); !strings.Contains(loc, "m=") || len(photos()) != 0 {
		t.Fatalf("принят не-фото файл: %s", loc)
	}
	// новый слайд
	post(true, map[string]string{"slot": "new", "caption": "  Форма\n85 ", "sub": "этап"}, png)
	c := cfg()
	if len(c.Slides) != 2 || c.Slides[1].Caption != "Форма 85" || !strings.HasPrefix(c.Slides[1].Src, "gallery/photo-") || len(photos()) != 1 {
		t.Fatalf("новый слайд не встал: %+v", c.Slides)
	}
	if c.Title != "Витрина" {
		t.Fatal("заголовок витрины потерян при записи")
	}
	// замена фото у мока: мок удаляется — он был только заглушкой
	post(true, map[string]string{"slot": "0", "caption": "Мок 2"}, png)
	if _, err := os.Stat(filepath.Join(gdir, "mock-1.svg")); !os.IsNotExist(err) {
		t.Fatal("заменённый мок остался на диске")
	}
	if c := cfg(); c.Slides[0].Caption != "Мок 2" || len(photos()) != 2 {
		t.Fatalf("замена фото: %+v, фото %d", c.Slides, len(photos()))
	}
	// удаление загруженного слайда удаляет и файл
	post(true, map[string]string{"slot": "1", "do": "remove"}, nil)
	if c := cfg(); len(c.Slides) != 1 || len(photos()) != 1 {
		t.Fatalf("удаление: %+v, фото %d", c.Slides, len(photos()))
	}
	// чужой путь в src не удаляется (защита от выхода из папки)
	if got := dropFile("../../SOUL.md", nil); got != nil {
		t.Fatal("dropFile вышел из папки витрины")
	}
	// файл, положенный руками, не трогается
	os.WriteFile(filepath.Join(gdir, "моё.jpg"), png, 0o644)
	if got := dropFile("gallery/моё.jpg", nil); got != nil {
		t.Fatal("dropFile удалил файл, положенный руками")
	}
}

func TestМокиВитриныУбираются(t *testing.T) {
	dir := vaultFixture(t)
	gdir := filepath.Join(dir, "viewer", "gallery")
	cfgJSON := `{"slides":[{"src":"gallery/mock-1.svg","caption":"А"},{"src":"gallery/photo-1.png","caption":"Б"},` +
		`{"src":"gallery/mock-2.svg","caption":"В"},{"src":"gallery/mock-2.svg","caption":"В-дубль"}]}`
	os.WriteFile(filepath.Join(gdir, "config.json"), []byte(cfgJSON), 0o644)
	for _, f := range []string{"mock-1.svg", "mock-2.svg", "photo-1.png"} {
		os.WriteFile(filepath.Join(gdir, f), []byte("x"), 0o644)
	}
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)
	post := func(fields map[string]string) string {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for k, v := range fields {
			mw.WriteField(k, v)
		}
		mw.Close()
		req := asAdmin(httptest.NewRequest(http.MethodPost, "/gallery", &buf))
		req.Header.Set("Content-Type", mw.FormDataContentType())
		rec := httptest.NewRecorder()
		handleGallery(rec, req)
		return rec.Header().Get("Location")
	}
	exists := func(f string) bool { _, err := os.Stat(filepath.Join(gdir, f)); return err == nil }

	// ✕ со старым src (витрину поправили в другой вкладке) — ничего не трогает
	post(map[string]string{"slot": "0", "src": "gallery/photo-1.png", "do": "remove"})
	if c, _ := readGallery(); len(c.Slides) != 4 {
		t.Fatal("убран слайд по устаревшему номеру")
	}
	// один из двух слайдов на mock-2: файл остаётся, пока на него ссылаются
	post(map[string]string{"slot": "3", "src": "gallery/mock-2.svg", "do": "remove"})
	if c, _ := readGallery(); len(c.Slides) != 3 || !exists("mock-2.svg") {
		t.Fatal("файл удалён, хотя на него ещё ссылается слайд")
	}
	if c, _ := readGallery(); c.Mocks() != 2 {
		t.Fatalf("примеров %d, ожидалось 2", c.Mocks())
	}
	// все примеры разом: настоящие фото остаются
	post(map[string]string{"do": "mocks"})
	c, _ := readGallery()
	if len(c.Slides) != 1 || c.Slides[0].Caption != "Б" || exists("mock-1.svg") || exists("mock-2.svg") || !exists("photo-1.png") {
		t.Fatalf("примеры убраны не так: %+v", c.Slides)
	}
	if loc := post(map[string]string{"do": "mocks"}); !strings.Contains(loc, "m=") {
		t.Fatal("повторная уборка не сказала, что примеров нет")
	}
}

func TestВиджетСегодня(t *testing.T) {
	dir := vaultFixture(t)
	head := "---\ntype: todo\nsummary: \"x\"\ntags: [дела]\nstatus: draft\n---\n"
	os.MkdirAll(filepath.Join(dir, "tasks"), 0o755)
	os.WriteFile(filepath.Join(dir, "tasks", "backlog.md"), []byte(head+"- [ ] Позвонить\n- [ ] Купить хлеб\n- [x] Сделанное\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "tasks", "day.md"), []byte(head+"## Текущий день\n\n- [ ] Позвонить\n- [ ] Своё @2026-01-02\n- [x] Готово\n"), 0o644)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)

	// без сессии — 401 и ни одного дела
	rec := httptest.NewRecorder()
	handleTodayJSON(rec, httptest.NewRequest(http.MethodGet, "/today.json", nil))
	if rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "Позвонить") {
		t.Fatalf("гостю отдан день: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handleTodayJSON(rec, asAdmin(httptest.NewRequest(http.MethodGet, "/today.json", nil)))
	var got widgetToday
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("не JSON: %v %s", err, rec.Body.String())
	}
	if len(got.Tasks) != 2 || got.Date == "" {
		t.Fatalf("не тот день: %+v", got)
	}
	if got.Tasks[0].URL != "/doc/tasks/backlog" || got.Tasks[1].URL != "/doc/tasks/day" || got.Tasks[1].Due != "2026-01-02" {
		t.Fatalf("дела ведут не туда: %+v", got.Tasks)
	}
	// окно «+» виджета: открытые дела, кроме уже стоящих на сегодня и закрытых
	if len(got.Pick) != 1 || got.Pick[0] != "Купить хлеб" {
		t.Fatalf("список выбора: %+v", got.Pick)
	}
}

func TestКалендарь(t *testing.T) {
	dir := vaultFixture(t)
	head := "---\ntype: todo\nsummary: \"x\"\ntags: [дела]\nstatus: draft\n---\n"
	os.MkdirAll(filepath.Join(dir, "tasks"), 0o755)
	os.WriteFile(filepath.Join(dir, "tasks", "backlog.md"), []byte(head+"# Бэклог\n\n## Высокий\n\n- [ ] Зубы @2026-10-12\n\n---\n\n### Как назначать\nсправка\n"), 0o644)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)

	// октябрь 2026 начинается в четверг: сетка с понедельника 28 сентября
	cal := buildCalendar(currentVault(), "2026-10-07", "2026-10", "2026-10-12")
	if cal.Title != "Октябрь 2026" || cal.Weeks[0][0].Date != "2026-09-28" || cal.Weeks[0][0].InMonth {
		t.Fatalf("сетка не с понедельника: %s %+v", cal.Title, cal.Weeks[0][0])
	}
	var d12, d7 calDay
	for _, w := range cal.Weeks {
		if len(w) != 7 {
			t.Fatal("неделя не из семи дней")
		}
		for _, d := range w {
			switch d.Date {
			case "2026-10-12":
				d12 = d
			case "2026-10-07":
				d7 = d
			}
		}
	}
	if d12.Due != 1 || !d12.Selected || !d7.Today {
		t.Fatalf("метки дней: 12-е %+v, 7-е %+v", d12, d7)
	}
	// журнал столпов из фикстуры: 2026-01-01 — оба держал
	jan := buildCalendar(currentVault(), "2026-10-07", "2026-01", "2026-01-01")
	if len(jan.Sel.Vows) != 2 || jan.Sel.Vows[0].Mark != "✓" {
		t.Fatalf("столпы дня: %+v", jan.Sel.Vows)
	}
	// дневник фикстуры: 2026-01-01 10:00
	if len(jan.Sel.Diary) != 1 || jan.Sel.Diary[0].Text != "старая запись" {
		t.Fatalf("дневник дня: %+v", jan.Sel.Diary)
	}
	// мусор в адресе — текущий месяц и сегодня
	if c := buildCalendar(currentVault(), "2026-10-07", "../x", "нет"); c.Sel.Date != "2026-10-07" {
		t.Fatalf("кривой адрес: %s", c.Sel.Date)
	}

	// «+ дело на этот день» — строка со сроком в раздел «По датам», до служебного хвоста
	form := url.Values{"date": {"2026-10-15"}, "text": {" Позвонить\nврачу "}}
	req := asAdmin(httptest.NewRequest(http.MethodPost, "/plan", strings.NewReader(form.Encode())))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handlePlan(rec, req)
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "day=2026-10-15") {
		t.Fatalf("возврат не в выбранный день: %s", loc)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "tasks", "backlog.md"))
	if !strings.Contains(string(raw), "## По датам 📅\n\n- [ ] Позвонить врачу @2026-10-15\n\n---") {
		t.Fatalf("дело на дату записано не туда:\n%s", raw)
	}
	if c := buildCalendar(currentVault(), "2026-10-07", "2026-10", "2026-10-15"); len(c.Sel.Tasks) != 1 {
		t.Fatal("календарь не видит запланированное")
	}
	// кривая дата не пишет ничего
	form.Set("date", "2026-02-31")
	req = asAdmin(httptest.NewRequest(http.MethodPost, "/plan", strings.NewReader(form.Encode())))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handlePlan(httptest.NewRecorder(), req)
	if raw2, _ := os.ReadFile(filepath.Join(dir, "tasks", "backlog.md")); string(raw2) != string(raw) {
		t.Fatal("записано на несуществующую дату")
	}
}

func TestВозвратТолькоНаСвойАдрес(t *testing.T) {
	for in, want := range map[string]string{
		"/?cal=2026-10#calendar": "/?cal=2026-10#calendar",
		"//evil.example/x":       "/",
		"https://evil.example":   "/",
		`/\evil.example`:         "/",
		"":                       "/",
	} {
		if got := localBack(in); got != want {
			t.Errorf("localBack(%q) = %q, ждали %q", in, got, want)
		}
	}
}

func TestВиджетКалендаря(t *testing.T) {
	dir := vaultFixture(t)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)

	rec := httptest.NewRecorder()
	handleCalendarJSON(rec, httptest.NewRequest(http.MethodGet, "/calendar.json?cal=2026-01", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("гостю отдан календарь: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	handleCalendarJSON(rec, asAdmin(httptest.NewRequest(http.MethodGet, "/calendar.json?cal=2026-01", nil)))
	var m widgetMonth
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("не JSON: %v", err)
	}
	if m.Month != "2026-01" || m.Prev != "2025-12" || m.Next != "2026-02" || m.Title != "Январь 2026" {
		t.Fatalf("месяц: %+v", m)
	}
	// 1 января 2026 — четверг: первая неделя с 29 декабря; в фикстуре в этот
	// день оба столпа держал и есть запись в дневнике
	if m.Weeks[0][0].Date != "2025-12-29" || m.Weeks[0][0].In {
		t.Fatalf("сетка: %+v", m.Weeks[0][0])
	}
	jan1 := m.Weeks[0][3]
	if jan1.Date != "2026-01-01" || !jan1.In || jan1.VowHeld != 2 || !jan1.Diary {
		t.Fatalf("1 января: %+v", jan1)
	}
}

func TestВиджетТела(t *testing.T) {
	dir := vaultFixture(t)
	os.MkdirAll(filepath.Join(dir, "facts"), 0o755)
	os.WriteFile(filepath.Join(dir, "facts", "body.md"), []byte("---\ntype: fact\nsummary: \"замеры\"\ntags: [здоровье]\nstatus: draft\nmetrics: body\n---\n"+
		"| Дата | Вес, кг | Талия, см |\n|---|---|---|\n| 2026-01-08 | 108 | 110 |\n| 2026-01-01 | 110 | 112 |\n| 2026-01-15 | | 109 |\n| 2026-01-22 | 106,5 | 108 |\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes", "goal.md"), []byte("---\ntype: note\nsummary: \"цель\"\ntags: [цели]\nstatus: draft\ntarget_kg: 85\n---\n# Цель\n"), 0o644)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)

	rec := httptest.NewRecorder()
	handleBodyJSON(rec, httptest.NewRequest(http.MethodGet, "/body.json", nil))
	if rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "108") {
		t.Fatalf("гостю отданы замеры: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	handleBodyJSON(rec, asAdmin(httptest.NewRequest(http.MethodGet, "/body.json", nil)))
	var b widgetBody
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("не JSON: %v", err)
	}
	// по дате, без пустой клетки 15-го; запятая — десятичная
	if b.Title != "Вес" || b.Unit != "кг" || len(b.Points) != 3 || b.Points[0].Value != 110 || b.Points[2].Value != 106.5 {
		t.Fatalf("ряд: %+v", b)
	}
	if b.Goal == nil || b.Goal.Target != 85 || b.Goal.Now != 106.5 || b.Goal.Left != 21.5 || b.Goal.Rate <= 0 {
		t.Fatalf("цель: %+v", b.Goal)
	}
}

func TestСессияПродлеваетсяПокаЕюПользуются(t *testing.T) {
	dir := vaultFixture(t)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)
	h := renewSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	call := func(token string) *http.Cookie {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		for _, c := range rec.Result().Cookies() {
			if c.Name == sessionCookie {
				return c
			}
		}
		return nil
	}
	// свежий билет (меньше суток) не перевыдаётся — без лишних Set-Cookie
	fresh, _ := newSession()
	if call(fresh) != nil {
		t.Fatal("свежий билет перевыдан")
	}
	// билет, которому осталось 3 дня, продлевается на полную неделю
	old := signSession(time.Now().Add(3 * 24 * time.Hour).Unix())
	c := call(old)
	if c == nil || !validSession(c.Value) || time.Until(c.Expires) < sessionTTL-time.Minute {
		t.Fatalf("живой билет не продлён: %+v", c)
	}
	// истёкший и поддельный — нет
	if call(signSession(time.Now().Add(-time.Hour).Unix())) != nil || call("99999999999.подделка") != nil {
		t.Fatal("продлён мёртвый или поддельный билет")
	}
}

func TestЗамерСДашборда(t *testing.T) {
	dir := vaultFixture(t)
	os.MkdirAll(filepath.Join(dir, "facts"), 0o755)
	path := filepath.Join(dir, "facts", "body.md")
	os.WriteFile(path, []byte("---\ntype: fact\nsummary: \"замеры\"\ntags: [здоровье]\nstatus: draft\nmetrics: body\ndemo: true\n---\n"+
		"| Дата | Вес, кг | Талия, см |\n|---|---|---|\n| 2026-01-01 | 110 | 112 |\n| 2026-01-08 | 108 | 110 |\n"), 0o644)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)
	post := func(vals url.Values) string {
		req := asAdmin(httptest.NewRequest(http.MethodPost, "/metric", strings.NewReader(vals.Encode())))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		handleMetric(rec, req)
		return rec.Header().Get("Location")
	}
	read := func() string { b, _ := os.ReadFile(path); return string(b) }

	if loc := post(url.Values{"do": {"add"}, "date": {"2026-01-15"}, "v0": {"107,4"}}); loc != "/#metrics" {
		t.Fatalf("возврат: %s", loc)
	}
	if !strings.Contains(read(), "| 2026-01-15 | 107.4 |  |") {
		t.Fatalf("замер не записан:\n%s", read())
	}
	if m := currentVault().Metrics; m == nil || len(m.Rows) != 3 {
		t.Fatal("модель не пересобрана")
	}
	before := read()
	for _, bad := range []url.Values{
		{"do": {"add"}, "v0": {"сто"}},
		{"do": {"add"}, "v0": {"-5"}},
		{"do": {"add"}},
		{"do": {"add"}, "date": {"2026-02-30"}, "v0": {"100"}},
	} {
		if loc := post(bad); !strings.Contains(loc, "m=") {
			t.Fatalf("принят плохой замер %v: %s", bad, loc)
		}
	}
	if read() != before {
		t.Fatal("плохой замер изменил карточку")
	}
	// чистка демо: остаётся первый замер, панель жива с одной точкой
	post(url.Values{"do": {"cleardemo"}})
	if strings.Contains(read(), "demo: true") || strings.Contains(read(), "2026-01-08") || !strings.Contains(read(), "2026-01-01") {
		t.Fatalf("чистка демо:\n%s", read())
	}
	if m := currentVault().Metrics; m == nil || len(m.Rows) != 1 || m.Demo {
		t.Fatalf("панель после чистки: %+v", m)
	}
}

func TestПокупки(t *testing.T) {
	dir := vaultFixture(t)
	head := "---\ntype: todo\nsummary: \"x\"\ntags: [дела]\nstatus: draft\n---\n"
	os.MkdirAll(filepath.Join(dir, "tasks"), 0o755)
	os.WriteFile(filepath.Join(dir, "tasks", "shopping.md"), []byte(head+"# Покупки\n\n## Еда\n\n- [ ] Хлеб\n- [x] Молоко\n\n## Быт\n\n_Пусто._\n\n---\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "tasks", "backlog.md"), []byte(head+"- [ ] Позвонить\n"), 0o644)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)
	v := currentVault()
	sv := shopping(v)
	if sv == nil || sv.Open != 1 || sv.Done != 1 || len(sv.Sections) != 2 {
		t.Fatalf("покупки: %+v", sv)
	}
	// покупки не дела: ни в учёте, ни в выборе на сегодня
	for _, i := range tasksOf(v).Items {
		if i.Src == shoppingCard {
			t.Fatal("покупка в учёте дел")
		}
	}
	if hasTask(flatten(pickableTasks(v)), "Хлеб") {
		t.Fatal("покупка в выборе дел")
	}
	form := url.Values{"text": {" Мыло "}, "group": {"Быт"}}
	req := asAdmin(httptest.NewRequest(http.MethodPost, "/shop", strings.NewReader(form.Encode())))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handleShop(httptest.NewRecorder(), req)
	raw, _ := os.ReadFile(filepath.Join(dir, "tasks", "shopping.md"))
	if !strings.Contains(string(raw), "## Быт\n\n- [ ] Мыло\n\n---") {
		t.Fatalf("покупка не в разделе:\n%s", raw)
	}
}

func TestДелоПереноситсяНаЗавтра(t *testing.T) {
	// срок: меняется хвост @дата, текст и отметка не трогаются
	raw := "- [ ] Сдать отчёт @2026-01-01\n- [x] Сдать отчёт\n- [ ] Позвонить\n"
	got, ok := setTaskDue(raw, "Сдать отчёт", "2026-01-05")
	if !ok || got != "- [ ] Сдать отчёт @2026-01-05\n- [x] Сдать отчёт\n- [ ] Позвонить\n" {
		t.Fatalf("срок не тот:\n%s", got)
	}
	if got, _ := setTaskDue(raw, "Позвонить", "2026-01-05"); !strings.Contains(got, "- [ ] Позвонить @2026-01-05") {
		t.Errorf("срок не дописан:\n%s", got)
	}

	dir := vaultFixture(t)
	day := "---\ntype: todo\nsummary: \"день\"\ntags: [дела]\nstatus: draft\n---\n# День\n\n## Текущий день\n\n- [ ] Позвонить врачу\n- [ ] Своё дело дня\n\n---\n"
	backlog := "---\ntype: todo\nsummary: \"бэклог\"\ntags: [дела]\nstatus: draft\n---\n# Бэклог\n\n## Высокий приоритет\n\n- [ ] Позвонить врачу\n- [ ] Отчёт @2026-01-01\n\n## По датам 📅\n\n"
	os.MkdirAll(filepath.Join(dir, "tasks"), 0o755)
	os.WriteFile(filepath.Join(dir, "tasks", "day.md"), []byte(day), 0o644)
	os.WriteFile(filepath.Join(dir, "tasks", "backlog.md"), []byte(backlog), 0o644)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)
	read := func(rel string) string {
		raw, _ := os.ReadFile(filepath.Join(dir, "tasks", rel))
		return string(raw)
	}
	want := "@" + tomorrow()

	// «Что горит»: срок дела — на завтра в его карточке
	if rec := postForm(handleTask, "/task", url.Values{"do": {"move"}, "src": {"tasks/backlog"}, "text": {"Отчёт"}}, true); rec.Header().Get("Location") != "/" {
		t.Fatalf("перенос вернул %q", rec.Header().Get("Location"))
	}
	if !strings.Contains(read("backlog.md"), "- [ ] Отчёт "+want) {
		t.Fatalf("срок не перенесён:\n%s", read("backlog.md"))
	}
	if rec := postForm(handleTask, "/task", url.Values{"do": {"move"}, "src": {"tasks/backlog"}, "text": {"Отчёт"}, "date": {"вчера"}}, true); !strings.Contains(rec.Header().Get("Location"), "m=") {
		t.Error("кривую дату приняли")
	}

	// «Сегодня»: дело из карточки — срок там и уходит из дня
	if rec := postForm(handleToday, "/today", url.Values{"do": {"later"}, "text": {"Позвонить врачу"}}, true); strings.Contains(rec.Header().Get("Location"), "m=") {
		t.Fatalf("на завтра: %q", rec.Header().Get("Location"))
	}
	if !strings.Contains(read("backlog.md"), "- [ ] Позвонить врачу "+want) || strings.Contains(read("day.md"), "Позвонить врачу") {
		t.Fatalf("дело из карточки не перенесено:\nбэклог:\n%s\nдень:\n%s", read("backlog.md"), read("day.md"))
	}
	// своё дело дня — в бэклог «По датам» на завтра
	postForm(handleToday, "/today", url.Values{"do": {"later"}, "text": {"Своё дело дня"}}, true)
	if !strings.Contains(read("backlog.md"), "## По датам 📅\n\n- [ ] Своё дело дня "+want) || strings.Contains(read("day.md"), "Своё дело дня") {
		t.Fatalf("своё дело не перенесено:\nбэклог:\n%s\nдень:\n%s", read("backlog.md"), read("day.md"))
	}

	// гостю — ни переноса, ни виджета
	before := read("backlog.md")
	postForm(handleTask, "/task", url.Values{"do": {"move"}, "src": {"tasks/backlog"}, "text": {"Отчёт"}, "date": {"2030-01-01"}}, false)
	if read("backlog.md") != before {
		t.Error("гость перенёс дело")
	}
	rec := httptest.NewRecorder()
	handleBurnJSON(rec, httptest.NewRequest(http.MethodGet, "/burn.json", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("гость получил «Что горит»: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	handleBurnJSON(rec, asAdmin(httptest.NewRequest(http.MethodGet, "/burn.json", nil)))
	if !strings.Contains(rec.Body.String(), `"text":"Отчёт"`) || !strings.Contains(rec.Body.String(), `"src":"tasks/backlog"`) {
		t.Errorf("в «Что горит» нет перенесённого дела: %s", rec.Body)
	}
}
