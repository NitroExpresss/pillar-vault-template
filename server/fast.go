// Голодание: считается по журналу еды (notes/food), отдельной кнопки «начал»
// нет — голод идёт с последней записанной еды. Приёмы ближе eatGap друг к
// другу — одно окно еды; промежутки между окнами — голодания.
//
// Протоколы (16:8, OMAD, 36 ч…) — таблица «## Протоколы» в карточке
// guides/intermittent-fasting-keto: один источник для справки и для кнопок.
// Выбор — в шапке журнала: fast_protocol и fast_window (начало окна еды).
// Без протокола цель — fast_goal, иначе 36 ч (ADF).
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	fastGuide       = "guides/intermittent-fasting-keto"
	fastDefaultGoal = 36.0
	fastDays        = 7             // окна за неделю — полоса виджета
	eatGap          = 3 * time.Hour // приёмы ближе — одно окно еды
	fastKeep        = 7             // прошлых голоданий в ответе
)

// fastStages — вехи голода, часы от последней еды. Подписи примерные: тело
// не переключается по часам, это ориентиры, а не физиология.
var fastStages = []struct {
	At   float64
	Name string
}{
	{0, "пищеварение"},
	{4, "сахар снижается"},
	{12, "жиросжигание"},
	{18, "кетоз"},
	{24, "аутофагия"},
	{48, "глубокий голод"},
}

// fastProto — протокол из таблицы карточки: часы голода и окно еды (0 —
// суточного окна нет, протокол многодневный).
type fastProto struct {
	ID     string  `json:"id"`
	Fast   float64 `json:"fast"`
	Window float64 `json:"window"`
	Note   string  `json:"note,omitempty"`
}

// fastPlan — окно еды на сегодня по протоколу: открыто ли и сколько до смены.
type fastPlan struct {
	Start string  `json:"start"` // ЧЧ:ММ
	End   string  `json:"end"`
	Open  bool    `json:"open"`
	In    float64 `json:"in"` // часов до открытия (закрыто) или до закрытия (открыто)
}

type fastWin struct {
	Start string `json:"start"` // RFC3339
	End   string `json:"end"`
	Kind  string `json:"kind"` // eat | fast | now — идущее голодание
}

type fastDone struct {
	Start string  `json:"start"`
	End   string  `json:"end"`
	Hours float64 `json:"hours"`
}

// fastView — GET /fast.json: дашборд и виджет «Голодание».
type fastView struct {
	Now     string     `json:"now"`
	Goal    float64    `json:"goal"`
	Last    string     `json:"last,omitempty"` // последняя еда; пусто — записей нет
	Hours   float64    `json:"hours"`          // голод идёт столько часов
	Pct     int        `json:"pct"`            // к цели, может быть больше 100
	Stage   string     `json:"stage"`
	Next    string     `json:"next,omitempty"`
	NextIn  float64    `json:"next_in,omitempty"` // часов до следующей вехи
	Windows []fastWin  `json:"windows"`
	Fasts   []fastDone `json:"fasts"` // прошлые, новые первыми
	Best    float64    `json:"best"`

	Protocol  string      `json:"protocol,omitempty"` // выбранный; пусто — своя цель
	Protocols []fastProto `json:"protocols"`
	Plan      *fastPlan   `json:"plan,omitempty"`
}

// fastProtocols — таблица «## Протоколы» карточки-гайда: первая колонка —
// имя, вторая — часы голода, третья — окно еды (— или пусто — нет), четвёртая — суть.
func fastProtocols(d *Doc) []fastProto {
	out := []fastProto{}
	if d == nil {
		return out
	}
	lines := strings.Split(normalizeEOL(d.Body), "\n")
	head, end := sectionSpan(lines, "## Протоколы")
	if head < 0 {
		return out
	}
	for _, l := range lines[head+1 : end] {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(l, "|"), "|")
		if len(cells) < 3 {
			continue
		}
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		var p fastProto
		if _, err := fmt.Sscan(strings.ReplaceAll(cells[1], ",", "."), &p.Fast); err != nil || p.Fast <= 0 {
			continue // шапка и черта таблицы
		}
		fmt.Sscan(strings.ReplaceAll(cells[2], ",", "."), &p.Window)
		p.ID = cells[0]
		if len(cells) > 3 {
			p.Note = cells[3]
		}
		out = append(out, p)
	}
	return out
}

// fastSetup — выбранный протокол (если он есть в таблице), цель в часах и
// начало окна еды из шапки журнала.
func fastSetup(food *Doc, protos []fastProto) (proto *fastProto, goal float64, window string) {
	goal = fastGoal(food)
	if food == nil {
		return nil, goal, ""
	}
	id := strings.TrimSpace(fmt.Sprint(food.FM["fast_protocol"]))
	for i := range protos {
		if protos[i].ID == id {
			proto, goal = &protos[i], protos[i].Fast
		}
	}
	if w, ok := food.FM["fast_window"].(string); ok && reClock.MatchString(w) {
		window = w
	}
	return proto, goal, window
}

var reClock = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// fastWindow — окно еды протокола вокруг now: сегодняшнее (или вчерашнее,
// если ещё идёт через полночь); открыто — сколько до закрытия, иначе — до
// ближайшего открытия.
func fastWindow(start string, length float64, now time.Time) *fastPlan {
	if length <= 0 || !reClock.MatchString(start) {
		return nil
	}
	t, _ := time.ParseInLocation("2006-01-02 15:04", ymd(now)+" "+start, now.Location())
	span := time.Duration(length * float64(time.Hour))
	if now.Before(t) && now.Before(t.Add(-24*time.Hour).Add(span)) {
		t = t.Add(-24 * time.Hour) // окно вчерашнего дня ещё открыто (через полночь)
	}
	p := &fastPlan{Start: start, End: t.Add(span).Format("15:04")}
	switch {
	case !now.Before(t) && now.Before(t.Add(span)):
		p.Open, p.In = true, hours(t.Add(span).Sub(now))
	case now.Before(t):
		p.In = hours(t.Sub(now))
	default:
		p.In = hours(t.Add(24 * time.Hour).Sub(now))
	}
	return p
}

var reFoodDay = regexp.MustCompile(`^## (\d{4}-\d{2}-\d{2})\s*$`)

// meals — все приёмы с временем из тела журнала, по порядку.
func meals(body string) []time.Time {
	var out []time.Time
	day := ""
	for _, l := range strings.Split(normalizeEOL(body), "\n") {
		l = strings.TrimSpace(l)
		if m := reFoodDay.FindStringSubmatch(l); m != nil {
			day = m[1]
			continue
		}
		if strings.HasPrefix(l, "## ") || l == "---" {
			day = ""
			continue
		}
		m := reFoodLine.FindStringSubmatch(l)
		if day == "" || m == nil {
			continue
		}
		hm := m[1]
		if len(hm) == 4 { // 9:30
			hm = "0" + hm
		}
		if t, err := time.ParseInLocation("2006-01-02 15:04", day+" "+hm, time.Local); err == nil {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

func fastGoal(d *Doc) float64 {
	if d == nil {
		return fastDefaultGoal
	}
	var g float64
	if _, err := fmt.Sscan(fmt.Sprint(d.FM["fast_goal"]), &g); err == nil && g > 0 && g <= 240 {
		return g
	}
	return fastDefaultGoal
}

func hours(d time.Duration) float64 { return math.Round(d.Hours()*10) / 10 }

// fasting — сводка голода на момент now.
func fasting(ms []time.Time, goal float64, now time.Time) fastView {
	v := fastView{Now: now.Format(time.RFC3339), Goal: goal, Windows: []fastWin{}, Fasts: []fastDone{}}
	var past []time.Time
	for _, t := range ms {
		if !t.After(now) {
			past = append(past, t)
		}
	}
	if len(past) == 0 {
		v.Stage = "нет записей еды"
		return v
	}
	// окна еды: [первый, последний] приём группы
	type span struct{ a, b time.Time }
	var eats []span
	for _, t := range past {
		if n := len(eats); n > 0 && t.Sub(eats[n-1].b) < eatGap {
			eats[n-1].b = t
		} else {
			eats = append(eats, span{t, t})
		}
	}
	from := now.Add(-fastDays * 24 * time.Hour)
	add := func(a, b time.Time, kind string) {
		if b.Before(from) {
			return
		}
		if a.Before(from) {
			a = from
		}
		v.Windows = append(v.Windows, fastWin{a.Format(time.RFC3339), b.Format(time.RFC3339), kind})
	}
	for i, e := range eats {
		add(e.a, e.b, "eat")
		if i+1 < len(eats) {
			nx := eats[i+1].a
			add(e.b, nx, "fast")
			v.Fasts = append(v.Fasts, fastDone{e.b.Format(time.RFC3339), nx.Format(time.RFC3339), hours(nx.Sub(e.b))})
		}
	}
	last := eats[len(eats)-1].b
	add(last, now, "now")

	sort.Slice(v.Fasts, func(i, j int) bool { return v.Fasts[i].End > v.Fasts[j].End })
	if len(v.Fasts) > fastKeep {
		v.Fasts = v.Fasts[:fastKeep]
	}
	for _, f := range v.Fasts {
		v.Best = math.Max(v.Best, f.Hours)
	}

	v.Last = last.Format(time.RFC3339)
	h := now.Sub(last).Hours()
	v.Hours = math.Round(h*10) / 10
	v.Pct = int(math.Round(h / goal * 100))
	for i, s := range fastStages {
		if h >= s.At {
			v.Stage = s.Name
			if i+1 < len(fastStages) {
				v.Next, v.NextIn = fastStages[i+1].Name, math.Round((fastStages[i+1].At-h)*10)/10
			} else {
				v.Next, v.NextIn = "", 0
			}
		}
	}
	return v
}

// fastNow — сводка голода с протоколом: для /fast.json и виджета.
func fastNow(v *Vault, now time.Time) fastView {
	d := v.Docs[foodCard]
	protos := fastProtocols(v.Docs[fastGuide])
	proto, goal, window := fastSetup(d, protos)
	var ms []time.Time
	if d != nil {
		ms = meals(d.Body)
	}
	out := fasting(ms, goal, now)
	out.Protocols = protos
	if proto != nil {
		out.Protocol = proto.ID
		out.Plan = fastWindow(window, proto.Window, now)
	}
	return out
}

// handleFastJSON — GET /fast.json.
func handleFastJSON(w http.ResponseWriter, r *http.Request) {
	if !widgetAuth(w, r) {
		return
	}
	json.NewEncoder(w).Encode(fastNow(currentVault(), time.Now()))
}

// handleFast — POST /fast: protocol — протокол из таблицы гайда (пусто — своя
// цель goal, часов), window — начало окна еды ЧЧ:ММ. Пишется в шапку журнала еды.
func handleFast(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	fail := func(msg string) { http.Redirect(w, r, "/?m="+urlQuery(msg)+"#w-food", http.StatusSeeOther) }
	v := currentVault()
	d := v.Docs[foodCard]
	if d == nil {
		fail("нет карточки " + foodCard)
		return
	}
	id := strings.TrimSpace(r.FormValue("protocol"))
	known := id == ""
	for _, p := range fastProtocols(v.Docs[fastGuide]) {
		known = known || p.ID == id
	}
	if !known {
		fail("нет такого протокола в " + fastGuide)
		return
	}
	window := strings.TrimSpace(r.FormValue("window"))
	if window != "" && !reClock.MatchString(window) {
		fail("начало окна — время ЧЧ:ММ")
		return
	}
	var goal float64
	if g := strings.TrimSpace(r.FormValue("goal")); g != "" {
		if _, err := fmt.Sscan(strings.ReplaceAll(g, ",", "."), &goal); err != nil || goal <= 0 || goal > 240 {
			fail("цель — часов, от 1 до 240")
			return
		}
	}
	what := "голодание: протокол " + id
	if id == "" {
		what = fmt.Sprintf("голодание: своя цель %g ч", goal)
	}
	if err := editCard(d.Path, what, func(raw string) (string, bool) {
		q := func(s string) string {
			if s == "" {
				return ""
			}
			return yamlQuote(s)
		}
		out, err := setFM(raw, "fast_protocol", q(id))
		if err == nil && (r.Form.Has("window") || id == "") {
			out, err = setFM(out, "fast_window", q(window))
		}
		if err == nil && goal > 0 {
			out, err = setFM(out, "fast_goal", fmt.Sprintf("%g", goal))
		}
		return out, err == nil
	}); err != nil {
		fail("не записано")
		return
	}
	http.Redirect(w, r, "/#w-food", http.StatusSeeOther)
}
