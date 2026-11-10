// Раздача APK для кнопки «Обновить приложение» и отказ виджетам кодом.
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

func TestAPKТолькоПоСессии(t *testing.T) {
	dir := vaultFixture(t)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)

	get := func(p string, admin bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		if admin {
			req = asAdmin(req)
		}
		rec := httptest.NewRecorder()
		handleApp(rec, req)
		return rec
	}

	// сборки на сервере ещё нет
	if rec := get("/app/version.json", true); rec.Code != http.StatusNotFound {
		t.Fatalf("без сборки: %d", rec.Code)
	}

	app := filepath.Join(dir, filepath.FromSlash(appDir))
	os.MkdirAll(app, 0o755)
	os.WriteFile(filepath.Join(app, appVersion), []byte(`{"code":7,"name":"1.0.7"}`), 0o644)
	os.WriteFile(filepath.Join(app, appAPK), []byte("PK-apk"), 0o644)
	os.WriteFile(filepath.Join(app, "secret.txt"), []byte("нет"), 0o644)

	if rec := get("/app/"+appAPK, false); rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "PK") {
		t.Fatalf("гостю отдан APK: %d", rec.Code)
	}
	if rec := get("/app/"+appVersion, true); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"code":7`) {
		t.Fatalf("версия: %d %q", rec.Code, rec.Body.String())
	}
	rec := get("/app/"+appAPK, true)
	if rec.Code != 200 || rec.Body.String() != "PK-apk" ||
		rec.Header().Get("Content-Type") != "application/vnd.android.package-archive" {
		t.Fatalf("APK: %d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	for _, p := range []string{"/app/secret.txt", "/app/../SOUL.md", "/app/"} {
		if rec := get(p, true); rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d — наружу только APK и версия", p, rec.Code)
		}
	}
}

// Протухшая сессия у виджета: раньше — тот же 303 на «/», что и успех, и
// виджет рапортовал «записано». Теперь — 401; браузеру по-прежнему редирект.
func TestВиджетБезСессииПолучает401(t *testing.T) {
	dir := vaultFixture(t)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)

	post := func(app bool) *httptest.ResponseRecorder {
		form := url.Values{"do": {"new"}, "text": {"дело"}}
		req := httptest.NewRequest(http.MethodPost, "/today", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if app {
			req.Header.Set(appHeader, "1")
		}
		rec := httptest.NewRecorder()
		handleToday(rec, req)
		return rec
	}
	if rec := post(true); rec.Code != http.StatusUnauthorized {
		t.Fatalf("виджет без сессии: %d", rec.Code)
	}
	if rec := post(false); rec.Code != http.StatusSeeOther {
		t.Fatalf("браузер без сессии: %d", rec.Code)
	}
}
