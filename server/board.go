// Данные дашборда для React-приложения одним ответом: календарь с выбранным
// днём, тело, дневник, «Что горит», колесо, учёт дел, витрина. Собирают их те
// же функции, что рисуют прежний шаблон (buildCalendar, burnZones, charts…):
// второго разбора в браузере нет. Отдаётся только то, что дашборд и так
// показывает, и только по сессии.
package main

import (
	"encoding/json"
	"net/http"
	"time"
)

type boardTask struct {
	Text string `json:"text"`
	Src  string `json:"src"` // карточка дела: в неё уходит «сделано» (POST /task)
	URL  string `json:"url"`
	Due  string `json:"due,omitempty"`
	Done bool   `json:"done,omitempty"`
}

func toBoardTask(t taskItem) boardTask {
	return boardTask{Text: t.Text, Src: t.Src, URL: docURL(t.Src), Due: t.Due, Done: t.Done}
}

type boardEntry struct {
	When string `json:"when"`
	Text string `json:"text"`
}

type boardPair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type boardDay struct {
	Date    string       `json:"date"`
	Label   string       `json:"label"`
	IsToday bool         `json:"isToday"`
	Tasks   []boardTask  `json:"tasks"`
	Vows    []boardPair  `json:"vows"` // value — ✓ или ✗
	Metrics []boardPair  `json:"metrics"`
	Diary   []boardEntry `json:"diary"`
}

type boardCalendar struct {
	widgetMonth
	Week []string `json:"week"`
	Day  boardDay `json:"day"`
}

type boardDue struct {
	boardTask
	When string `json:"when"` // «вчера», «через 3 дня»
}

type boardZone struct {
	Key   string     `json:"key"` // over, now, next
	Title string     `json:"title"`
	Items []boardDue `json:"items"`
}

type boardSphere struct {
	Name  string      `json:"name"`
	Score int         `json:"score"`
	Max   int         `json:"max"`
	Pct   int         `json:"pct"`
	Low   bool        `json:"low,omitempty"`
	Goals []string    `json:"goals,omitempty"`
	Cards []boardLink `json:"cards,omitempty"` // цели, планы, проекты сферы
}

type boardLink struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type boardField struct {
	Idx  int    `json:"idx"`
	Name string `json:"name"`
	Last string `json:"last,omitempty"`
}

type boardBody struct {
	Title  string       `json:"title"`
	URL    string       `json:"url"`
	Demo   bool         `json:"demo,omitempty"`
	Charts []string     `json:"charts"` // готовые SVG от charts.go: числа и подписи экранированы
	Goal   *widgetGoal  `json:"goal,omitempty"`
	Fields []boardField `json:"fields"`
}

type boardGroup struct {
	Name  string      `json:"name"`
	Items []boardTask `json:"items"`
}

type boardTasks struct {
	Open   int          `json:"open"`
	Done   int          `json:"done"`
	Pct    int          `json:"pct"`
	Groups []boardGroup `json:"groups"`
}

type boardData struct {
	Today     string        `json:"today"`
	Calendar  boardCalendar `json:"calendar"`
	Body      *boardBody    `json:"body,omitempty"`
	Diary     []boardEntry  `json:"diary"` // последние записи, свежая первой
	Deadlines []boardZone   `json:"deadlines"`
	Wheel     []boardSphere `json:"wheel"`
	Tasks     *boardTasks   `json:"tasks,omitempty"`
	Gallery   *boardGallery `json:"gallery,omitempty"`
	Pins      []widgetPin   `json:"pins"`  // важные заметки (pin: true)
	Goals     []goalView    `json:"goals"` // цели как OKR: результаты, время, темп
}

type boardGallery struct {
	*galleryConfig
	Mocks int `json:"mocks"` // слайдов-примеров: «Убрать примеры» видна, пока они есть
}

// boardDayOf — срез одного дня календаря: дела, столпы, замеры, дневник.
func boardDayOf(sel calDetail) boardDay {
	d := boardDay{Date: sel.Date, Label: sel.Label, IsToday: sel.IsToday,
		Tasks: []boardTask{}, Vows: []boardPair{}, Metrics: []boardPair{}, Diary: []boardEntry{}}
	for _, t := range sel.Tasks {
		d.Tasks = append(d.Tasks, toBoardTask(t))
	}
	for _, x := range sel.Vows {
		d.Vows = append(d.Vows, boardPair{Name: x.Name, Value: x.Mark})
	}
	for _, x := range sel.Metrics {
		d.Metrics = append(d.Metrics, boardPair{Name: x.Name, Value: x.Value})
	}
	for _, e := range sel.Diary {
		d.Diary = append(d.Diary, boardEntry{When: e.When, Text: e.Text})
	}
	return d
}

// handleDayJSON — GET /day.json?date=ГГГГ-ММ-ДД: окно дня виджета «Календарь».
func handleDayJSON(w http.ResponseWriter, r *http.Request) {
	if !widgetAuth(w, r) {
		return
	}
	date := r.URL.Query().Get("date")
	if _, err := time.Parse("2006-01-02", date); err != nil || !reISODate.MatchString(date) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"нет такой даты"}`))
		return
	}
	cal := buildCalendar(currentVault(), ymd(time.Now()), date[:7], date)
	json.NewEncoder(w).Encode(boardDayOf(cal.Sel))
}

// handleBoard — GET /api/board?cal=ГГГГ-ММ&day=ГГГГ-ММ-ДД
func handleBoard(w http.ResponseWriter, r *http.Request) {
	if !widgetAuth(w, r) {
		return
	}
	v := currentVault()
	today := ymd(time.Now())
	out := boardData{Today: today, Diary: []boardEntry{}, Deadlines: []boardZone{}, Wheel: []boardSphere{}}

	cal := buildCalendar(v, today, r.URL.Query().Get("cal"), r.URL.Query().Get("day"))
	out.Calendar = boardCalendar{
		widgetMonth: widgetMonth{Title: cal.Title, Month: cal.Month, Prev: cal.PrevMonth, Next: cal.NextMonth},
		Week:        cal.Week,
		Day:         boardDayOf(cal.Sel),
	}
	for _, wk := range cal.Weeks {
		row := make([]widgetDay, 0, 7)
		for _, d := range wk {
			row = append(row, widgetDay{Date: d.Date, Day: d.Day, In: d.InMonth, Today: d.Today,
				Due: d.Due, Diary: d.Diary, Metric: d.Metric, VowHeld: d.VowHeld, VowBroke: d.VowBroke})
		}
		out.Calendar.Weeks = append(out.Calendar.Weeks, row)
	}

	if m := v.Metrics; m != nil {
		b := &boardBody{Title: m.Title, URL: docURL(m.Source), Demo: m.Demo, Charts: []string{}, Fields: []boardField{}}
		for _, svg := range charts(m) {
			b.Charts = append(b.Charts, string(svg))
		}
		for _, f := range metricFields(m) {
			b.Fields = append(b.Fields, boardField{Idx: f.Idx, Name: f.Name, Last: f.Last})
		}
		if g := v.Goal; g != nil {
			b.Goal = &widgetGoal{Now: g.Now, Target: g.Target, Start: g.Start, Left: g.Left, Pct: g.Pct, Rate: g.Rate}
		}
		out.Body = b
	}

	if d := v.Docs[diaryCard]; d != nil {
		all := diaryEntries(d.Body)
		for i := len(all) - 1; i >= 0 && len(out.Diary) < 3; i-- {
			out.Diary = append(out.Diary, boardEntry{When: all[i].When, Text: all[i].Text})
		}
	}

	out.Deadlines = boardZones(v, today)

	for _, s := range v.Spheres {
		bs := boardSphere{Name: s.Name, Score: s.Score, Max: s.Max, Pct: s.Pct, Low: s.Low, Goals: s.Goals}
		for _, d := range s.Cards {
			bs.Cards = append(bs.Cards, boardLink{Title: d.Title, URL: docURL(d.ID)})
		}
		out.Wheel = append(out.Wheel, bs)
	}

	if ts := tasksOf(v); ts != nil {
		bt := &boardTasks{Open: ts.Open, Done: ts.Done, Pct: donePct(ts), Groups: []boardGroup{}}
		for _, g := range groupTasks(ts) {
			bg := boardGroup{Name: g.Name, Items: []boardTask{}}
			for _, t := range g.Items {
				bg.Items = append(bg.Items, toBoardTask(t))
			}
			bt.Groups = append(bt.Groups, bg)
		}
		out.Tasks = bt
	}

	if g := loadGallery(); g != nil {
		out.Gallery = &boardGallery{galleryConfig: g, Mocks: g.Mocks()}
	}
	out.Pins = pinsOf(v)
	out.Goals = goalsOf(v, today)
	json.NewEncoder(w).Encode(out)
}
