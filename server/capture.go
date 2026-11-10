// Захват мысли с виджета: строка падает в шлюз inbox/ (карточка формы
// захвата), а дальше — обычный Ingest по уставу. Ссылка — в раздел ссылок,
// остальное — в новые идеи; у строки дата, чтобы при разборе было видно,
// откуда и когда она пришла.
package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	captureCard     = "inbox/capture-ideas"
	captureIdeas    = "## 💡 Новые идеи"
	captureLinks    = "## 🔗 Ссылки для изучения"
	maxCaptureRunes = 1000
)

// handleCapture — POST /capture: text.
func handleCapture(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	fail := func(msg string) { http.Redirect(w, r, urlWithMsg("/", msg), http.StatusSeeOther) }
	text := strings.Join(strings.Fields(r.FormValue("text")), " ")
	switch {
	case text == "":
		fail("пустая мысль")
		return
	case utf8.RuneCountInString(text) > maxCaptureRunes:
		fail("слишком длинно — это уже карточка, а не мысль")
		return
	}
	d := currentVault().Docs[captureCard]
	if d == nil {
		fail("нет карточки " + captureCard)
		return
	}
	section := captureIdeas
	if strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://") {
		section = captureLinks
	}
	item := fmt.Sprintf("- %s %s", time.Now().Format("2006-01-02 15:04"), text)
	if err := editCard(d.Path, "захват: "+text, func(raw string) (string, bool) {
		return insertInSection(raw, section, item), true
	}); err != nil {
		fail("не записано")
		return
	}
	markOK(w)
	http.Redirect(w, r, urlWithMsg("/", "в inbox"), http.StatusSeeOther)
}
