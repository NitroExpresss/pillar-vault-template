// Свежий APK для кнопки «Обновить приложение».
//
// Репозиторий приватный: релиз на GitHub телефон без входа не скачает, а
// вшивать токен в APK — отдавать его любому, кто APK вскроет. Поэтому APK
// раздаёт сам сервер и только по сессии. Сюда он попадает так: CI кладёт
// сборку в ветку android-apk, deploy.sh раз в такт достаёт её в server/app/
// (вне git). Здесь — только отдача.
package main

import (
	"net/http"
	"os"
	"path/filepath"
)

const (
	// appHeader ставят запросы виджетов (VaultApi.java): по нему сервер
	// отвечает на отказ кодом, а не редиректом для браузера.
	appHeader = "X-Pillar-App"

	appDir     = "server/app"
	appAPK     = "pillar-vault.apk"
	appVersion = "version.json" // {"code": 42, "name": "1.0.42"}
)

// handleApp — /app/version.json и /app/pillar-vault.apk. Больше ничего из
// каталога наружу не идёт: имена — закрытый список.
func handleApp(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if !isAdmin(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"нужен вход"}`))
		return
	}
	var name, ctype string
	switch r.URL.Path {
	case "/app/" + appVersion:
		name, ctype = appVersion, "application/json; charset=utf-8"
	case "/app/" + appAPK:
		name, ctype = appAPK, "application/vnd.android.package-archive"
	default:
		http.NotFound(w, r)
		return
	}
	full := filepath.Join(root, filepath.FromSlash(appDir), name)
	f, err := os.Open(full)
	if err != nil {
		http.NotFound(w, r) // сборки на сервере ещё нет
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ctype)
	http.ServeContent(w, r, name, st.ModTime(), f)
}
