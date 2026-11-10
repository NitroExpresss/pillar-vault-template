// Журнал входов и смена пароля без перезапуска: что пишется, кто может
// сменить ключ и что старые сессии после этого мертвы.
package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withAccess — хранилище с папкой server/ и чистым счётчиком попыток.
func withAccess(t *testing.T, pass string) string {
	t.Helper()
	dir := vaultFixture(t)
	if err := os.MkdirAll(filepath.Join(dir, "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	hash, _ := hashPassword(pass)
	withVault(t, dir, hash)
	oldAttempts, oldStamp := attempts, hashFileStamp
	attempts = newAttemptStore()
	t.Cleanup(func() { attempts, hashFileStamp = oldAttempts, oldStamp })
	return dir
}

func postForm(h http.HandlerFunc, path string, form url.Values, admin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 14) Chrome/129.0 PillarVaultApp/1.4")
	if admin {
		req = asAdmin(req)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func sessionFrom(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			return c
		}
	}
	return nil
}

func TestЖурналВходов(t *testing.T) {
	withAccess(t, "правильный-ключ")

	postForm(handleLogin, "/login", url.Values{"password": {"мимо"}}, false)
	if rec := postForm(handleLogin, "/login", url.Values{"password": {"правильный-ключ"}}, false); sessionFrom(rec) == nil {
		t.Fatal("верный ключ не впустил")
	}

	logins := readLogins(10)
	if len(logins) != 2 || logins[0].Result != loginOK || logins[1].Result != loginDenied {
		t.Fatalf("журнал не тот (свежие сверху): %+v", logins)
	}
	if d := logins[0].Device(); d != "приложение Android" {
		t.Errorf("устройство распознано как %q", d)
	}

	// журнал — только под сессией
	rec := httptest.NewRecorder()
	handleAccessAPI(rec, httptest.NewRequest(http.MethodGet, "/api/access", nil))
	if rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), loginDenied) {
		t.Fatalf("гостю отдали журнал: код %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	handleAccessAPI(rec, asAdmin(httptest.NewRequest(http.MethodGet, "/api/access", nil)))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, loginDenied) || !strings.Contains(body, "приложение Android") {
		t.Fatalf("журнал не показан: код %d", rec.Code)
	}
	if strings.Contains(body, "правильный-ключ") || strings.Contains(body, "мимо") {
		t.Fatal("в журнал утёк пароль")
	}
}

func TestСменаПароляСВеба(t *testing.T) {
	dir := withAccess(t, "старый-ключ")
	oldToken, _ := newSession()

	// гость не меняет ничего, даже зная старый ключ
	rec := postForm(handleAccess, "/access", url.Values{"old": {"старый-ключ"}, "new": {"новый-ключ"}, "repeat": {"новый-ключ"}}, false)
	if rec.Code != http.StatusSeeOther || !validSession(oldToken) {
		t.Fatal("гость сменил пароль")
	}

	bad := []url.Values{
		{"old": {"не-тот"}, "new": {"новый-ключ"}, "repeat": {"новый-ключ"}},
		{"old": {"старый-ключ"}, "new": {"коротко"}, "repeat": {"коротко"}},
		{"old": {"старый-ключ"}, "new": {"новый-ключ"}, "repeat": {"другой-ключ"}},
	}
	for _, form := range bad {
		postForm(handleAccess, "/access", form, true)
		if !validSession(oldToken) {
			t.Fatalf("пароль сменился по негодной форме %v", form)
		}
	}

	rec = postForm(handleAccess, "/access", url.Values{"old": {"старый-ключ"}, "new": {"новый-ключ"}, "repeat": {"новый-ключ"}}, true)
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, url.QueryEscape("ключ сменён")) {
		t.Fatalf("смена не прошла: %s", loc)
	}
	if validSession(oldToken) {
		t.Fatal("старая сессия пережила смену пароля")
	}
	if c := sessionFrom(rec); c == nil || !validSession(c.Value) {
		t.Fatal("устройство, сменившее ключ, выставлено за дверь")
	}
	if ok, _ := verifyPassword("новый-ключ", passHash()); !ok {
		t.Fatal("новый ключ не действует")
	}
	// и переживёт перезапуск: файл тот же, что читает сервер на старте
	if got := passHashFromFile(); got != passHash() {
		t.Fatalf("в %s не новый хэш", filepath.Join(dir, "server", ".passhash"))
	}
	if logins := readLogins(1); len(logins) != 1 || logins[0].Result != loginPassChanged {
		t.Fatalf("смена не попала в журнал: %+v", logins)
	}
}

func TestПарольИзФайлаБезПерезапуска(t *testing.T) {
	dir := withAccess(t, "прежний-ключ")
	file := filepath.Join(dir, "server", ".passhash")
	hashFileStamp = passHashStamp()
	before := passHash()

	// битый файл замок не трогает
	os.WriteFile(file, []byte("мусор\n"), 0o600)
	reloadPassHash()
	if passHash() != before {
		t.Fatal("битый файл заменил пароль")
	}
	// пустой — тоже: опечатка не должна выключать вход
	os.WriteFile(file, []byte("\n"), 0o600)
	reloadPassHash()
	if passHash() != before {
		t.Fatal("пустой файл снял пароль")
	}

	next, _ := hashPassword("свежий-ключ")
	os.WriteFile(file, []byte(next+"\n"), 0o600)
	reloadPassHash()
	if passHash() != next {
		t.Fatal("новый файл не подхвачен")
	}

	// из окружения файл не следим
	hashFromEnv = true
	t.Cleanup(func() { hashFromEnv = false })
	other, _ := hashPassword("ещё-ключ")
	os.WriteFile(file, []byte(other+"   \n"), 0o600)
	reloadPassHash()
	if passHash() != next {
		t.Fatal("пароль из окружения перебит файлом")
	}
}

func TestЖурналВходовНеРастётВечно(t *testing.T) {
	dir := withAccess(t, "ключ-ключ")
	full := filepath.Join(dir, filepath.FromSlash(loginLogFile))
	line := "2026-01-01T00:00:00Z\t10.0.0.1\t" + loginDenied + "\t" + strings.Repeat("x", 200) + "\n"
	os.WriteFile(full, []byte(strings.Repeat(line, loginLogMax/len(line)+10)), 0o600)
	postForm(handleLogin, "/login", url.Values{"password": {"мимо"}}, false)
	raw, _ := os.ReadFile(full)
	if n := strings.Count(string(raw), "\n"); n != loginLogKeep {
		t.Fatalf("после обрезки %d строк, ожидалось %d", n, loginLogKeep)
	}
	if logins := readLogins(1); len(logins) != 1 || logins[0].IP != "192.0.2.1" {
		t.Fatalf("свежая запись потерялась при обрезке: %+v", logins)
	}
}
