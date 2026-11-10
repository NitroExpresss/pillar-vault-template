// Разбор входящих без агента: мысли из «Захвата» лежат строками в
// inbox/capture-ideas по разделам. Каждую — в дела, в карточку (форма
// новой карточки, после неё строка уходит отсюда) или удалить. Шлюз
// остаётся шлюзом: ничего не попадает в хранилище мимо формы по уставу.
package main

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

const inboxTaskSection = "## Средний приоритет 🟡"

// дата и время, которые «Захват» ставит перед мыслью
var reCaptureStamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}( \d{2}:\d{2})?\s+`)

type inboxItem struct {
	Section string `json:"section"`
	Text    string `json:"text"` // строка как есть, без «- »: по ней действие находит строку
	Clean   string `json:"clean"`
	When    string `json:"when,omitempty"`
	Link    bool   `json:"link,omitempty"`
}

// inboxItems — мысли карточки захвата: пункты списков под заголовками «##»
// до черты; дела-чекбоксы и цитаты — не мысли.
func inboxItems(raw string) []inboxItem {
	out := []inboxItem{}
	section := ""
	_, body := parseFrontmatter(normalizeEOL(raw))
	for _, l := range strings.Split(body, "\n") {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "## "):
			section = strings.TrimSpace(strings.TrimPrefix(t, "## "))
			continue
		case t == "---" && section != "":
			return out
		case section == "" || !strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "- ["):
			continue
		}
		text := strings.TrimSpace(strings.TrimPrefix(t, "- "))
		if text == "" {
			continue
		}
		it := inboxItem{Section: section, Text: text, Clean: text}
		if m := reCaptureStamp.FindString(text); m != "" {
			it.When = strings.TrimSpace(m)
			it.Clean = strings.TrimSpace(text[len(m):])
		}
		it.Link = strings.HasPrefix(it.Clean, "http://") || strings.HasPrefix(it.Clean, "https://")
		out = append(out, it)
	}
	return out
}

// removeInboxLine убирает первую строку-мысль с этим текстом.
func removeInboxLine(raw, text string) (string, bool) {
	lines := strings.Split(normalizeEOL(raw), "\n")
	for i, l := range lines {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, "- ") && strings.TrimSpace(strings.TrimPrefix(t, "- ")) == text {
			return strings.Join(append(lines[:i:i], lines[i+1:]...), "\n"), true
		}
	}
	return raw, false
}

type inboxPage struct {
	Items []inboxItem `json:"items"`
	Files []docRef    `json:"files"` // другие карточки в inbox/ — их разбирают правкой
	Src   string      `json:"src"`
}

func inboxOf(v *Vault) inboxPage {
	out := inboxPage{Items: []inboxItem{}, Files: []docRef{}, Src: captureCard}
	if d := v.Docs[captureCard]; d != nil {
		if raw, err := readCard(d.Path); err == nil {
			out.Items = inboxItems(raw)
		}
	}
	for _, d := range v.Index {
		if d.TopFolder == "inbox" && d.ID != captureCard {
			out.Files = append(out.Files, refOf(d))
		}
	}
	return out
}

// handleInboxAPI — GET /api/inbox: мысли и прочие карточки входящих.
func handleInboxAPI(w http.ResponseWriter, r *http.Request) {
	if !widgetAuth(w, r) {
		return
	}
	json.NewEncoder(w).Encode(inboxOf(currentVault()))
}

// handleInbox — POST /inbox: do=remove — убрать мысль (разобрана или не
// нужна), do=task — сделать из неё дело в бэклоге и убрать.
func handleInbox(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet { // экран «Входящие» — приложение
		handleRoot(w, r)
		return
	}
	if !requirePost(w, r) {
		return
	}
	fail := func(msg string) { http.Redirect(w, r, urlWithMsg("/", msg), http.StatusSeeOther) }
	v := currentVault()
	d := v.Docs[captureCard]
	text := strings.TrimSpace(r.FormValue("text"))
	if d == nil {
		fail("нет карточки " + captureCard)
		return
	}
	var item *inboxItem
	if raw, err := readCard(d.Path); err == nil {
		for _, it := range inboxItems(raw) {
			if it.Text == text {
				it := it
				item = &it
				break
			}
		}
	}
	if item == nil {
		fail("такой мысли во входящих нет")
		return
	}
	switch r.FormValue("do") {
	case "task":
		task := strings.Join(strings.Fields(item.Clean), " ")
		if len([]rune(task)) > maxTaskRunes {
			fail("слишком длинно для дела — сделайте карточку")
			return
		}
		plan := v.Docs[planCard]
		if plan == nil {
			fail("нет карточки " + planCard)
			return
		}
		if err := editCard(plan.Path, "из входящих в дела: "+task, func(raw string) (string, bool) {
			return insertInSection(raw, inboxTaskSection, "- [ ] "+task), true
		}); err != nil {
			fail("дело не записано")
			return
		}
	case "remove":
	default:
		http.Error(w, "неизвестное действие", http.StatusBadRequest)
		return
	}
	if err := editCard(d.Path, "разобрано из входящих: "+item.Clean, func(raw string) (string, bool) {
		return removeInboxLine(raw, text)
	}); err != nil {
		fail("не убрано из входящих")
		return
	}
	http.Redirect(w, r, "/inbox", http.StatusSeeOther)
}

// inboxCount — сколько мыслей ждёт разбора: число в меню.
func inboxCount(v *Vault) int {
	d := v.Docs[captureCard]
	if d == nil {
		return 0
	}
	raw, err := readCard(d.Path)
	if err != nil {
		return 0
	}
	return len(inboxItems(raw))
}
