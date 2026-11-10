// Важные заметки: карточка закрепляется строкой `pin: true` во frontmatter
// (правило — в SOUL.md). Закреплённые видны виджетом на дашборде и на
// рабочем столе Android; закрепить можно кнопкой на карточке или руками
// агента — источник один, сама карточка.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

const maxPins = 12 // больше — уже не «важное», а второй список дел

// pinned — закреплённые карточки: свежеобновлённые первыми, затем по заголовку.
func pinned(v *Vault) []*Doc {
	out := []*Doc{}
	for _, d := range v.Index {
		if isPinned(d) && d.Status != "archived" {
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ui, uj := fmString(out[i].FM, "updated"), fmString(out[j].FM, "updated")
		if ui != uj {
			return ui > uj
		}
		return out[i].Title < out[j].Title
	})
	if len(out) > maxPins {
		out = out[:maxPins]
	}
	return out
}

func isPinned(d *Doc) bool { return fmString(d.FM, "pin") == "true" }

// setPin ставит или снимает `pin: true` во frontmatter, не трогая остальное.
// Снять — убрать строку: «pin: false» в карточке ничего не говорит.
func setPin(raw string, on bool) (string, error) {
	if on {
		return setFM(raw, "pin", "true")
	}
	return setFM(raw, "pin", "")
}

// setFM ставит верхний ключ frontmatter (value "" — убрать строку), не
// трогая остальное: вложенные строки edges, порядок, переводы строк.
func setFM(raw, key, value string) (string, error) {
	eol := "\n"
	if strings.Contains(raw, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.Split(raw, eol)
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return "", errors.New("у карточки нет frontmatter")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return "", errors.New("frontmatter не закрыт")
	}
	head := []string{}
	for _, l := range lines[1:end] {
		if !strings.HasPrefix(strings.TrimSpace(l), key+":") || strings.HasPrefix(l, " ") {
			head = append(head, l)
		}
	}
	if value != "" {
		head = append(head, key+": "+value)
	}
	out := append([]string{lines[0]}, head...)
	return strings.Join(append(out, lines[end:]...), eol), nil
}

// handlePin — POST /pin: id, on=1 (закрепить) или on=0 (открепить).
func handlePin(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	d := currentVault().Docs[r.FormValue("id")]
	back := "/"
	if d != nil {
		back = docURL(d.ID)
	}
	fail := func(msg string) { http.Redirect(w, r, urlWithMsg(back, msg), http.StatusSeeOther) }
	if d == nil {
		fail("нет такой карточки")
		return
	}
	on := r.FormValue("on") == "1"
	if on == isPinned(d) { // уже так — повтор нажатия не ошибка
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	if on && len(pinned(currentVault())) >= maxPins {
		fail(fmt.Sprintf("закреплено уже %d — сначала открепите лишнее", maxPins))
		return
	}
	what := "закреплено: " + d.ID
	if !on {
		what = "откреплено: " + d.ID
	}
	var bad error
	if err := editCard(d.Path, what, func(raw string) (string, bool) {
		next, err := setPin(raw, on)
		bad = err
		return next, err == nil
	}); err != nil {
		if bad != nil {
			err = bad
		}
		fail("не записано: " + err.Error())
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

type widgetPin struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"`
	URL     string `json:"url"`
}

func pinsOf(v *Vault) []widgetPin {
	out := []widgetPin{}
	for _, d := range pinned(v) {
		out = append(out, widgetPin{ID: d.ID, Title: d.Title, Summary: d.Summary, URL: docURL(d.ID)})
	}
	return out
}

// handlePinsJSON — GET /pins.json: виджет «Важное» на рабочем столе.
func handlePinsJSON(w http.ResponseWriter, r *http.Request) {
	if !widgetAuth(w, r) {
		return
	}
	json.NewEncoder(w).Encode(struct {
		Items []widgetPin `json:"items"`
	}{pinsOf(currentVault())})
}

// handleArchive — POST /archive: id, on=1 — в архив (status: archived,
// закрепление снимается: архив — не «важное»), on=0 — вернуть (draft).
// Карточка остаётся на месте: из поиска и «Важного» уходит, ссылки живы.
func handleArchive(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	d := currentVault().Docs[r.FormValue("id")]
	if d == nil {
		http.Redirect(w, r, urlWithMsg("/", "нет такой карточки"), http.StatusSeeOther)
		return
	}
	back := docURL(d.ID)
	on := r.FormValue("on") == "1"
	status, what := "draft", "из архива: "+d.ID
	if on {
		status, what = "archived", "в архив: "+d.ID
	}
	var bad error
	if err := editCard(d.Path, what, func(raw string) (string, bool) {
		next, err := setFM(raw, "status", status)
		if err == nil && on {
			next, err = setFM(next, "pin", "")
		}
		bad = err
		return next, err == nil
	}); err != nil {
		if bad != nil {
			err = bad
		}
		http.Redirect(w, r, urlWithMsg(back, "не записано: "+err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}
