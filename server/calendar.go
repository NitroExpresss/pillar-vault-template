// Календарь на дашборде: месяц сеткой и всё, что в хранилище привязано к
// дате, — сроки дел, журнал столпов, дневник, замеры тела. Отдельного
// хранилища событий нет и не нужно: календарь только читает карточки,
// а «дело на этот день» пишет обычной строкой со сроком в бэклог.
package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// планы на дату ложатся в бэклог отдельным разделом
const (
	planCard    = "tasks/backlog"
	planSection = "## По датам 📅"
)

var reYearMonth = regexp.MustCompile(`^\d{4}-\d{2}$`)

type calDay struct {
	Date     string
	Day      int
	InMonth  bool
	Today    bool
	Selected bool
	Due      int  // дел со сроком (открытых)
	Diary    bool // есть запись в дневнике
	Metric   bool // есть замер тела
	VowHeld  int  // столпов держал
	VowBroke int  // столпов порвал
	URL      string
}

type calVow struct{ Name, Mark string }
type calMetric struct{ Name, Value string }

// calDetail — выбранный день под сеткой.
type calDetail struct {
	Date    string
	Label   string // «12 октября, суббота»
	URL     string // сюда же вернуться после отметки дела
	IsToday bool
	IsPast  bool
	Tasks   []taskItem
	Diary   []DiaryEntry
	Vows    []calVow
	Metrics []calMetric
}

type calendarView struct {
	Month     string // 2026-10
	PrevMonth string
	NextMonth string
	Title     string // «Октябрь 2026»
	PrevURL   string
	NextURL   string
	TodayURL  string
	Week      []string // пн … вс
	Weeks     [][]calDay
	Sel       calDetail
}

var ruMonthsNom = [...]string{"Январь", "Февраль", "Март", "Апрель", "Май", "Июнь",
	"Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь"}

func calURL(month, day string) string {
	q := url.Values{"cal": {month}}
	if day != "" {
		q.Set("day", day)
	}
	return "/?" + q.Encode() + "#calendar"
}

// buildCalendar — месяц month (ГГГГ-ММ) с выбранным днём day. Пустые или
// кривые значения — текущий месяц и сегодня.
func buildCalendar(v *Vault, today, month, day string) *calendarView {
	if !reISODate.MatchString(day) {
		day = ""
	}
	if !reYearMonth.MatchString(month) {
		month = today[:7]
		if day != "" {
			month = day[:7]
		}
	}
	first, err := time.Parse("2006-01", month)
	if err != nil {
		first, _ = time.Parse("2006-01", today[:7])
		month = today[:7]
	}
	if day == "" || day[:7] != month {
		day = month + "-01"
		if today[:7] == month {
			day = today
		}
	}

	ev := collectDated(v)
	prev, next := first.AddDate(0, -1, 0).Format("2006-01"), first.AddDate(0, 1, 0).Format("2006-01")
	cv := &calendarView{
		Month: month, PrevMonth: prev, NextMonth: next,
		Title:    ruMonthsNom[first.Month()-1] + " " + first.Format("2006"),
		PrevURL:  calURL(prev, ""),
		NextURL:  calURL(next, ""),
		TodayURL: calURL(today[:7], today),
		Week:     []string{"пн", "вт", "ср", "чт", "пт", "сб", "вс"},
	}
	// сетка с понедельника: от понедельника недели, где первое число,
	// до воскресенья недели, где последнее
	start := first.AddDate(0, 0, -((int(first.Weekday()) + 6) % 7))
	last := first.AddDate(0, 1, -1)
	end := last.AddDate(0, 0, (7-int(last.Weekday()))%7)
	var week []calDay
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		date := ymd(d)
		e := ev.on(date)
		cd := calDay{
			Date: date, Day: d.Day(), InMonth: d.Month() == first.Month(),
			Today: date == today, Selected: date == day,
			Diary: len(e.diary) > 0, Metric: len(e.metrics) > 0,
			URL: calURL(month, date),
		}
		for _, t := range e.tasks {
			if !t.Done {
				cd.Due++
			}
		}
		for _, m := range e.vows {
			switch m.Mark {
			case vowHeld:
				cd.VowHeld++
			case vowBroke:
				cd.VowBroke++
			}
		}
		if !cd.InMonth { // день соседнего месяца — бледный, нажатие переводит в его месяц
			cd.URL = calURL(date[:7], date)
		}
		week = append(week, cd)
		if len(week) == 7 {
			cv.Weeks = append(cv.Weeks, week)
			week = nil
		}
	}

	e := ev.on(day)
	cv.Sel = calDetail{Date: day, Label: dateRu(day), URL: calURL(month, day), IsToday: day == today, IsPast: day < today,
		Tasks: e.tasks, Diary: e.diary, Vows: e.vows, Metrics: e.metrics}
	return cv
}

type datedEvents struct {
	tasks   []taskItem
	diary   []DiaryEntry
	vows    []calVow
	metrics []calMetric
}

// collectDated — всё датированное в хранилище, по дням.
func collectDated(v *Vault) datedIndex {
	out := datedIndex{}
	at := func(date string) *datedEvents {
		if out[date] == nil {
			out[date] = &datedEvents{}
		}
		return out[date]
	}
	if v.Tasks != nil {
		for _, t := range v.Tasks.Items {
			if t.Due != "" && t.Src != dayCard {
				at(t.Due).tasks = append(at(t.Due).tasks, t)
			}
		}
	}
	if v.VowLog != nil {
		for _, row := range v.VowLog.Rows {
			for i, h := range v.VowLog.Header {
				if i > 0 && i < len(row) && row[i] != "" {
					at(row[0]).vows = append(at(row[0]).vows, calVow{Name: h, Mark: row[i]})
				}
			}
		}
	}
	if d := v.Docs[diaryCard]; d != nil {
		for _, e := range diaryEntries(d.Body) {
			at(e.When[:10]).diary = append(at(e.When[:10]).diary, e)
		}
	}
	if m := v.Metrics; m != nil {
		for _, r := range m.Rows {
			for i, val := range r.Values {
				if val != nil && i+1 < len(m.Columns) {
					at(r.Date).metrics = append(at(r.Date).metrics, calMetric{Name: m.Columns[i+1], Value: num(*val)})
				}
			}
		}
	}
	return out
}

// diaryEntries — все записи дневника: заголовок «## дата [время]» и первый
// абзац под ним (lastDiaryEntry берёт из них последнюю).
func diaryEntries(body string) []DiaryEntry {
	out := []DiaryEntry{}
	for _, p := range splitH2(body) {
		head, rest, _ := strings.Cut(p, "\n")
		when := strings.TrimSpace(head)
		if !reISOPrefix.MatchString(when) {
			continue
		}
		text := strings.TrimSpace(rest)
		if j := strings.Index(text, "\n\n"); j >= 0 {
			text = strings.TrimSpace(text[:j])
		}
		if text != "" {
			out = append(out, DiaryEntry{When: when, Text: text})
		}
	}
	return out
}

// handlePlan — «+ дело на этот день» из календаря: строка со сроком в
// бэклог, раздел «По датам». Оттуда дело видно в «Что горит», в календаре
// и в «Выбрать из всех дел».
func handlePlan(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	date := r.FormValue("date")
	if _, err := time.Parse("2006-01-02", date); err != nil || !reISODate.MatchString(date) {
		http.Redirect(w, r, urlWithMsg("/", "нет такой даты"), http.StatusSeeOther)
		return
	}
	back := calURL(date[:7], date)
	fail := func(msg string) { // сообщение — в адрес, перед якорем календаря
		http.Redirect(w, r, strings.Replace(back, "#", "&m="+url.QueryEscape(msg)+"#", 1), http.StatusSeeOther)
	}
	text := strings.Join(strings.Fields(r.FormValue("text")), " ")
	switch {
	case text == "":
		fail("пустое дело")
		return
	case utf8.RuneCountInString(text) > maxTaskRunes:
		fail("слишком длинное дело")
		return
	}
	d := currentVault().Docs[planCard]
	if d == nil {
		fail("нет карточки " + planCard)
		return
	}
	item := "- [ ] " + text + " @" + date
	if err := editCard(d.Path, "на "+date+": "+text, func(raw string) (string, bool) {
		return insertInSection(raw, planSection, item), true
	}); err != nil {
		fail("дело не записано")
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

type datedIndex map[string]*datedEvents

// on — события дня; день без событий — пустой набор, а не nil.
func (ix datedIndex) on(date string) *datedEvents {
	if e := ix[date]; e != nil {
		return e
	}
	return &datedEvents{}
}
