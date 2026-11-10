// Данные для виджетов Android на рабочем столе.
//
// Единственное место, где сервер отдаёт данные, а не страницы: виджеты
// Android («Сегодня», «Календарь», «Тело») не умеют HTML. Отдаётся только то,
// что и так видно в тех же блоках дашборда, — и только по той же сессии.
package main

import (
	"encoding/json"
	"math"
	"net/http"
	"time"
)

type widgetTask struct {
	Text string `json:"text"`
	Due  string `json:"due,omitempty"`
	Late bool   `json:"late,omitempty"` // срок прошёл
	URL  string `json:"url"`            // карточка, где дело живёт: туда ведёт нажатие на текст
}

type widgetToday struct {
	Date  string       `json:"date"` // «7 октября, среда»
	Tasks []widgetTask `json:"tasks"`
	Pick  []string     `json:"pick"` // открытые дела, которые можно поставить на сегодня
}

func handleTodayJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	if !isAdmin(r) {
		// виджет по коду ответа поймёт, что пора войти в приложении
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"нужен вход"}`))
		return
	}
	v := currentVault()
	today := ymd(time.Now())
	out := widgetToday{Date: dateRu(today), Tasks: []widgetTask{}, Pick: []string{}}
	for _, t := range todayList(v) {
		// даты ISO сравниваются строкой
		out.Tasks = append(out.Tasks, widgetTask{Text: t.Text, Due: t.Due, Late: t.Due != "" && t.Due < today, URL: docURL(taskHome(v, t))})
	}
	// для окна «+» виджета — тот же список, что «Выбрать из всех дел» на сайте
	for _, t := range flatten(pickableTasks(v)) {
		out.Pick = append(out.Pick, t.Text)
	}
	json.NewEncoder(w).Encode(out)
}

// ─────────────────── виджет «Календарь» ───────────────────

type widgetDay struct {
	Date     string `json:"date"`
	Day      int    `json:"day"`
	In       bool   `json:"in"` // этого месяца, а не соседнего
	Today    bool   `json:"today,omitempty"`
	Due      int    `json:"due,omitempty"`
	Diary    bool   `json:"diary,omitempty"`
	Metric   bool   `json:"metric,omitempty"`
	VowHeld  int    `json:"vowHeld,omitempty"`
	VowBroke int    `json:"vowBroke,omitempty"`
}

type widgetMonth struct {
	Title string        `json:"title"` // «Октябрь 2026»
	Month string        `json:"month"` // 2026-10
	Prev  string        `json:"prev"`
	Next  string        `json:"next"`
	Weeks [][]widgetDay `json:"weeks"`
}

// handleCalendarJSON — месяц для виджета: та же сетка и те же метки, что
// у календаря на дашборде (buildCalendar), только списком, а не HTML.
func handleCalendarJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	if !isAdmin(r) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"нужен вход"}`))
		return
	}
	today := ymd(time.Now())
	cal := buildCalendar(currentVault(), today, r.URL.Query().Get("cal"), "")
	out := widgetMonth{Title: cal.Title, Month: cal.Month, Prev: cal.PrevMonth, Next: cal.NextMonth}
	for _, wk := range cal.Weeks {
		row := make([]widgetDay, 0, 7)
		for _, d := range wk {
			row = append(row, widgetDay{Date: d.Date, Day: d.Day, In: d.InMonth, Today: d.Today,
				Due: d.Due, Diary: d.Diary, Metric: d.Metric, VowHeld: d.VowHeld, VowBroke: d.VowBroke})
		}
		out.Weeks = append(out.Weeks, row)
	}
	json.NewEncoder(w).Encode(out)
}

// ─────────────────── виджет «Тело» ───────────────────

type widgetPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

type widgetGoal struct {
	Now    float64 `json:"now"`
	Target float64 `json:"target"`
	Start  float64 `json:"start"`
	Left   float64 `json:"left"`
	Pct    int     `json:"pct"`
	Rate   float64 `json:"rate"` // кг в неделю, плюс — вниз
}

type widgetBody struct {
	Title  string        `json:"title"` // «Вес»
	Unit   string        `json:"unit"`  // «кг»
	Demo   bool          `json:"demo,omitempty"`
	Points []widgetPoint `json:"points"`
	Goal   *widgetGoal   `json:"goal,omitempty"`
	// для окна «+ замер» виджета: колонки таблицы после даты и последние значения
	Columns []string `json:"columns"`
	Last    []string `json:"last"`
}

// handleBodyJSON — первый ряд замеров (вес) и полоса к цели: то же, что
// панель «Тело» на дашборде, для виджета-графика. Пустые клетки пропущены.
func handleBodyJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	if !isAdmin(r) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"нужен вход"}`))
		return
	}
	v := currentVault()
	out := widgetBody{Points: []widgetPoint{}, Columns: []string{}, Last: []string{}}
	if m := v.Metrics; m != nil {
		if len(m.Columns) > 1 {
			out.Columns = m.Columns[1:]
		}
		for i := range out.Columns {
			last := ""
			for k := len(m.Rows) - 1; k >= 0 && last == ""; k-- { // последнее непустое
				if i < len(m.Rows[k].Values) && m.Rows[k].Values[i] != nil {
					last = num(*m.Rows[k].Values[i])
				}
			}
			out.Last = append(out.Last, last)
		}
		col := "ряд 1"
		if len(m.Columns) > 1 {
			col = m.Columns[1]
		}
		out.Title, out.Unit, out.Demo = shortName(col), unitOf(col), m.Demo
		for _, row := range m.Rows {
			if len(row.Values) > 0 && row.Values[0] != nil {
				out.Points = append(out.Points, widgetPoint{Date: row.Date, Value: *row.Values[0]})
			}
		}
	}
	if g := v.Goal; g != nil {
		out.Goal = &widgetGoal{Now: g.Now, Target: g.Target, Start: g.Start, Left: g.Left, Pct: g.Pct,
			Rate: math.Round(g.Rate*10) / 10}
	}
	json.NewEncoder(w).Encode(out)
}

// ─────────────────── виджет «Столпы» ───────────────────

type widgetVow struct {
	Name   string `json:"name"`
	Rule   string `json:"rule"`
	Streak int    `json:"streak"`
	Record int    `json:"record"`
	Mark   string `json:"mark"` // ✓, ✗ или пусто — за сегодня
}

type widgetVows struct {
	Date string      `json:"date"`
	Held int         `json:"held"` // сегодня отмечено «держал»
	Vows []widgetVow `json:"vows"`
}

// handleVowsJSON — столпы со стриком и сегодняшней отметкой. Стрик считается
// на момент запроса, а не сборки модели: после полуночи виджет не должен
// показывать вчерашнее «сегодня».
func handleVowsJSON(w http.ResponseWriter, r *http.Request) {
	if !widgetAuth(w, r) {
		return
	}
	v := currentVault()
	now := time.Now()
	out := widgetVows{Date: dateRu(ymd(now)), Vows: []widgetVow{}}
	for _, vw := range v.Vows {
		cur, rec, mark := vw.Streak, vw.Record, vw.Mark
		if v.VowLog != nil {
			cur, rec, mark = v.VowLog.streakOf(vw.Name, now)
		}
		if mark == vowHeld {
			out.Held++
		}
		out.Vows = append(out.Vows, widgetVow{Name: vw.Name, Rule: vw.Rule, Streak: cur, Record: rec, Mark: mark})
	}
	json.NewEncoder(w).Encode(out)
}

// ─────────────────── виджет «Покупки» ───────────────────

type widgetShopItem struct {
	Text  string `json:"text"`
	Group string `json:"group"`
}

type widgetShop struct {
	Src      string           `json:"src"` // карточка: в неё уходит «куплено» (POST /task)
	Items    []widgetShopItem `json:"items"`
	Sections []string         `json:"sections"` // куда класть новую покупку (POST /shop)
	Done     int              `json:"done"`
}

// handleShopJSON — что купить, по разделам карточки покупок.
func handleShopJSON(w http.ResponseWriter, r *http.Request) {
	if !widgetAuth(w, r) {
		return
	}
	out := widgetShop{Src: shoppingCard, Items: []widgetShopItem{}, Sections: []string{}}
	if sv := shopping(currentVault()); sv != nil {
		out.Sections, out.Done = sv.Sections, sv.Done
		for _, g := range sv.Groups {
			for _, t := range g.Open {
				out.Items = append(out.Items, widgetShopItem{Text: t.Text, Group: g.Name})
			}
		}
	}
	json.NewEncoder(w).Encode(out)
}

// widgetAuth — общая шапка JSON виджетов. Без сессии — 401: по нему
// виджет поймёт, что пора войти в приложении.
func widgetAuth(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	if !isAdmin(r) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"нужен вход"}`))
		return false
	}
	return true
}
