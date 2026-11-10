// Цели как OKR: карточка type: goal со сроком (deadline) и таблицей
// «Ключевые результаты» — строка «Цель» задаёт цели метрик, строки с датой —
// показания. Колонка с тем же именем, что в журнале замеров («Вес, кг»),
// берёт показания оттуда: вес вписывается один раз — виджетом «Замер».
//
// Прогресс метрики — доля пути от первого показания к цели; время — доля
// пути от старта к сроку. Темп — их разница: виден отстающий результат
// задолго до дедлайна. Формат — в SOUL.md.
package main

import (
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const goalKRHeading = "## Ключевые результаты"

type goalPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
	Pct   float64 `json:"pct"` // доля пути к цели, 0–100 (может выйти за края)
}

type goalKR struct {
	Name    string      `json:"name"`
	Unit    string      `json:"unit,omitempty"`
	Start   float64     `json:"start"`
	Current float64     `json:"current"`
	Target  float64     `json:"target"`
	Pct     float64     `json:"pct"`
	Points  []goalPoint `json:"points"`
	From    string      `json:"from,omitempty"` // показания из журнала замеров
	Col     int         `json:"col"`            // колонка таблицы (без даты): v<col> в POST /goal
}

type goalView struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	URL      string   `json:"url"`
	Start    string   `json:"start"`
	Deadline string   `json:"deadline,omitempty"`
	DaysLeft int      `json:"daysLeft"`
	TimePct  float64  `json:"timePct"` // прошло времени, 0–100; без срока — 0
	Pct      float64  `json:"pct"`     // средний прогресс результатов
	Pace     string   `json:"pace"`    // ahead | on | behind | done | nodeadline | nodata
	Gap      float64  `json:"gap"`     // прогресс минус время, п.п.
	KRs      []goalKR `json:"krs"`
	Tasks    [2]int   `json:"tasks"` // сделано, всего — дела в самой карточке
}

// goalTable — таблица под заголовком «Ключевые результаты»: колонки, строка
// «Цель» и строки с датами.
func goalTable(body string) (cols []string, target []*float64, rows []metricRow) {
	lines := strings.Split(normalizeEOL(body), "\n")
	in := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "## ") {
			in = t == goalKRHeading
			continue
		}
		if !in || !strings.HasPrefix(t, "|") {
			if in && cols != nil && t != "" {
				break // таблица кончилась
			}
			continue
		}
		cells := []string{}
		for _, c := range strings.Split(strings.TrimSuffix(strings.TrimPrefix(t, "|"), "|"), "|") {
			cells = append(cells, strings.TrimSpace(c))
		}
		rule := true
		for _, c := range cells {
			rule = rule && reTableRule.MatchString(c)
		}
		if rule {
			continue
		}
		if cols == nil {
			cols = cells
			continue
		}
		vals := make([]*float64, 0, len(cells)-1)
		for _, v := range cells[1:] {
			vals = append(vals, parseNum(v))
		}
		switch {
		case strings.EqualFold(cells[0], "Цель"):
			target = vals
		case reISODate.MatchString(cells[0]):
			rows = append(rows, metricRow{Date: cells[0], Values: vals})
		}
	}
	return cols, target, rows
}

func parseNum(s string) *float64 {
	s = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), ",", "."), " ", "")
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return nil
	}
	return &n
}

// goalOf считает цель на сегодня; nil — у карточки нет таблицы результатов
// и дел (её ещё не перевели в OKR).
func goalOf(v *Vault, d *Doc, today string) *goalView {
	cols, target, rows := goalTable(d.Body)
	g := &goalView{ID: d.ID, Title: d.Title, URL: docURL(d.ID), KRs: []goalKR{},
		Start: fmString(d.FM, "start"), Deadline: fmString(d.FM, "deadline")}
	if !reISODate.MatchString(g.Start) {
		g.Start = d.Created
	}
	if !reISODate.MatchString(g.Deadline) {
		g.Deadline = ""
	}
	// дела цели — только в разделах «## Дела…»: повторяющиеся задачи и
	// контрольные точки продвижение не меряют (вес и так виден в результатах)
	inTasks := false
	for _, l := range strings.Split(d.Body, "\n") {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, "## ") {
			inTasks = strings.HasPrefix(t, "## Дела")
			continue
		}
		if _, _, done, ok := parseTaskLine(l); ok && inTasks {
			g.Tasks[1]++
			if done {
				g.Tasks[0]++
			}
		}
	}
	if cols == nil && g.Tasks[1] == 0 {
		return nil
	}

	for i := 1; i < len(cols); i++ {
		if i-1 >= len(target) || target[i-1] == nil {
			continue // без цели метрика — не результат, а заметка
		}
		kr := goalKR{Name: shortName(cols[i]), Unit: unitOf(cols[i]), Target: *target[i-1], Points: []goalPoint{}, Col: i - 1}
		series := map[string]float64{}
		for _, r := range rows {
			if i-1 < len(r.Values) && r.Values[i-1] != nil {
				series[r.Date] = *r.Values[i-1]
			}
		}
		// та же колонка в журнале замеров — показания оттуда (свои строки цели важнее)
		if m := v.Metrics; m != nil {
			for j := 1; j < len(m.Columns); j++ {
				if !strings.EqualFold(strings.TrimSpace(m.Columns[j]), strings.TrimSpace(cols[i])) {
					continue
				}
				kr.From = m.Source
				for _, r := range m.Rows {
					if _, own := series[r.Date]; !own && j-1 < len(r.Values) && r.Values[j-1] != nil && r.Date >= g.Start {
						series[r.Date] = *r.Values[j-1]
					}
				}
			}
		}
		dates := make([]string, 0, len(series))
		for dt := range series {
			dates = append(dates, dt)
		}
		sort.Strings(dates)
		if len(dates) == 0 {
			g.KRs = append(g.KRs, kr) // цель есть, показаний ещё нет
			continue
		}
		kr.Start, kr.Current = series[dates[0]], series[dates[len(dates)-1]]
		for _, dt := range dates {
			kr.Points = append(kr.Points, goalPoint{Date: dt, Value: series[dt], Pct: pathPct(kr.Start, series[dt], kr.Target)})
		}
		kr.Pct = pathPct(kr.Start, kr.Current, kr.Target)
		g.KRs = append(g.KRs, kr)
	}

	// старт не задан явно — путь начинается с первого показания, если оно
	// раньше даты создания карточки (цель завели, а показания вписали задним числом)
	if fmString(d.FM, "start") == "" {
		for _, kr := range g.KRs {
			if len(kr.Points) > 0 && kr.Points[0].Date < g.Start {
				g.Start = kr.Points[0].Date
			}
		}
	}

	// прогресс цели — среднее по результатам с показаниями, а без них — по делам
	n, sum := 0, 0.0
	for _, kr := range g.KRs {
		if len(kr.Points) > 0 {
			sum += clampPct(kr.Pct)
			n++
		}
	}
	switch {
	case n > 0:
		g.Pct = round1(sum / float64(n))
	case g.Tasks[1] > 0:
		g.Pct = round1(100 * float64(g.Tasks[0]) / float64(g.Tasks[1]))
	}

	t0, err0 := time.Parse("2006-01-02", g.Start)
	now, _ := time.Parse("2006-01-02", today)
	if g.Deadline != "" && err0 == nil {
		t1, _ := time.Parse("2006-01-02", g.Deadline)
		g.DaysLeft = int(math.Ceil(t1.Sub(now).Hours() / 24))
		if span := t1.Sub(t0).Hours(); span > 0 {
			g.TimePct = round1(clampPct(100 * now.Sub(t0).Hours() / span))
		}
	}
	g.Gap = round1(g.Pct - g.TimePct)
	switch {
	case g.Pct >= 100:
		g.Pace = "done"
	case n == 0 && g.Tasks[1] == 0:
		g.Pace = "nodata"
	case g.Deadline == "":
		g.Pace = "nodeadline"
	case g.Gap >= 10:
		g.Pace = "ahead"
	case g.Gap <= -10:
		g.Pace = "behind"
	default:
		g.Pace = "on"
	}
	return g
}

// pathPct — доля пути от start к target, которую прошло значение cur.
// Работает в обе стороны: вес вниз, доход вверх.
func pathPct(start, cur, target float64) float64 {
	if start == target {
		if cur == target {
			return 100
		}
		return 0
	}
	return round1(100 * (cur - start) / (target - start))
}

func clampPct(p float64) float64 { return math.Max(0, math.Min(100, p)) }

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// goalsOf — цели хранилища: открытые, ближайший срок первым.
func goalsOf(v *Vault, today string) []goalView {
	out := []goalView{}
	for _, d := range v.Index {
		if d.Type != "goal" || d.Status == "archived" {
			continue
		}
		if g := goalOf(v, d, today); g != nil {
			out = append(out, *g)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Deadline, out[j].Deadline
		if (a == "") != (b == "") {
			return a != ""
		}
		return a < b
	})
	return out
}

// handleGoalsJSON — GET /goals.json: виджет «Цели» на рабочем столе.
func handleGoalsJSON(w http.ResponseWriter, r *http.Request) {
	if !widgetAuth(w, r) {
		return
	}
	json.NewEncoder(w).Encode(struct {
		Today string     `json:"today"`
		Goals []goalView `json:"goals"`
	}{ymd(time.Now()), goalsOf(currentVault(), ymd(time.Now()))})
}
