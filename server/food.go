// Еда: журнал notes/food.md — что съедено за день. Раздел «## ГГГГ-ММ-ДД»
// на день, строка «- ЧЧ:ММ что съел» на приём. Пишут дашборд, виджет «Еда»
// и плитка «Что ел» в «Захвате»; показывается сегодняшний день.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	foodCard     = "notes/food"
	maxFoodRunes = 300
	foodRecent   = 3  // дней до сегодняшнего — «последние дни» в блоке на главной
	foodHistory  = 60 // дней в «Истории»
)

type foodItem struct {
	Time string `json:"time"`
	Text string `json:"text"`
}

// foodDayView — день журнала: «последние дни» и «История».
type foodDayView struct {
	Date  string     `json:"date"`
	Items []foodItem `json:"items"`
}

// widgetFood — GET /food.json: сегодняшний день журнала и несколько дней
// до него; ?history=1 — ещё и все дни, новые первыми.
type widgetFood struct {
	Src     string        `json:"src"`
	Date    string        `json:"date"`
	Items   []foodItem    `json:"items"`
	Recent  []foodDayView `json:"recent"`
	History []foodDayView `json:"history,omitempty"`
}

// foodDays — все дни журнала с приёмами, новые первыми.
func foodDays(body string) []foodDayView {
	var out []foodDayView
	for _, l := range strings.Split(normalizeEOL(body), "\n") {
		if m := reFoodDay.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			if items := foodDay(body, m[1]); len(items) > 0 {
				out = append(out, foodDayView{m[1], items})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	return out
}

// foodRecentDays — дни журнала за n суток до day (без него), новые первыми.
func foodRecentDays(days []foodDayView, day string, n int) []foodDayView {
	out := []foodDayView{}
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return out
	}
	from := ymd(t.AddDate(0, 0, -n))
	for _, d := range days {
		if d.Date < day && d.Date >= from {
			out = append(out, d)
		}
	}
	return out
}

var reFoodLine = regexp.MustCompile(`^- (\d{1,2}:\d{2})\s+(.+)$`)

// foodDay — приёмы пищи за день из тела карточки.
func foodDay(body, day string) []foodItem {
	out := []foodItem{}
	lines := strings.Split(normalizeEOL(body), "\n")
	head, end := sectionSpan(lines, "## "+day)
	if head < 0 {
		return out
	}
	for _, l := range lines[head+1 : end] {
		l = strings.TrimSpace(l)
		if m := reFoodLine.FindStringSubmatch(l); m != nil {
			out = append(out, foodItem{Time: m[1], Text: m[2]})
		} else if t := strings.TrimSpace(strings.TrimPrefix(l, "- ")); strings.HasPrefix(l, "- ") && t != "" {
			out = append(out, foodItem{Text: t}) // записано руками без времени
		}
	}
	return out
}

// foodAdd — дописать строку в конец раздела дня; нет раздела — новый в конец файла.
func foodAdd(raw, day, item string) string {
	lines := strings.Split(normalizeEOL(raw), "\n")
	head, end := sectionSpan(lines, "## "+day)
	if head < 0 {
		return strings.TrimRight(strings.Join(lines, "\n"), " \t\n") + "\n\n## " + day + "\n\n" + item + "\n"
	}
	at := end
	for at > head+1 && strings.TrimSpace(lines[at-1]) == "" {
		at-- // пустые строки в хвосте раздела остаются после новой
	}
	next := append(append(append([]string{}, lines[:at]...), item), lines[at:]...)
	return strings.Join(next, "\n")
}

// foodRemove — убрать строку из раздела дня (ошибся — передумал).
func foodRemove(raw, day, line string) (string, bool) {
	lines := strings.Split(normalizeEOL(raw), "\n")
	head, end := sectionSpan(lines, "## "+day)
	if head < 0 {
		return raw, false
	}
	for i := head + 1; i < end; i++ {
		if strings.TrimSpace(lines[i]) == line {
			return strings.Join(append(append([]string{}, lines[:i]...), lines[i+1:]...), "\n"), true
		}
	}
	return raw, false
}

// handleFood — POST /food: text — записать приём пищи; do=remove (time, text) —
// убрать сегодняшнюю запись.
func handleFood(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	fail := func(msg string) { http.Redirect(w, r, "/?m="+urlQuery(msg)+"#food", http.StatusSeeOther) }
	d := currentVault().Docs[foodCard]
	if d == nil {
		fail("нет карточки " + foodCard)
		return
	}
	now := time.Now()
	day := ymd(now)
	text := strings.Join(strings.Fields(r.FormValue("text")), " ")
	if r.FormValue("do") == "remove" {
		line := "- " + text
		if t := r.FormValue("time"); t != "" {
			line = "- " + t + " " + text
		}
		if err := editCard(d.Path, "еда: убрано "+text, func(raw string) (string, bool) {
			return foodRemove(raw, day, line)
		}); err != nil {
			fail("не убрано")
			return
		}
		http.Redirect(w, r, "/#food", http.StatusSeeOther)
		return
	}
	switch {
	case text == "":
		fail("пусто — что съели?")
		return
	case utf8.RuneCountInString(text) > maxFoodRunes:
		fail("слишком длинно")
		return
	}
	item := fmt.Sprintf("- %s %s", now.Format("15:04"), text)
	if err := editCard(d.Path, "еда: "+text, func(raw string) (string, bool) {
		return foodAdd(raw, day, item), true
	}); err != nil {
		fail("не записано")
		return
	}
	markOK(w)
	http.Redirect(w, r, urlWithMsg("/", "записано")+"#food", http.StatusSeeOther)
}

// handleFoodJSON — GET /food.json: сегодня в журнале еды (дашборд и виджет).
func handleFoodJSON(w http.ResponseWriter, r *http.Request) {
	if !widgetAuth(w, r) {
		return
	}
	day := ymd(time.Now())
	out := widgetFood{Src: foodCard, Date: day, Items: []foodItem{}, Recent: []foodDayView{}}
	if d := currentVault().Docs[foodCard]; d != nil {
		out.Items = foodDay(d.Body, day)
		days := foodDays(d.Body)
		out.Recent = foodRecentDays(days, day, foodRecent)
		if r.URL.Query().Get("history") != "" {
			if len(days) > foodHistory {
				days = days[:foodHistory]
			}
			out.History = append([]foodDayView{}, days...)
		}
	}
	json.NewEncoder(w).Encode(out)
}
