// Модель хранилища в памяти — то, по чему сервер рисует страницы.
//
// Здесь же разбор дашборда: столпы, дневник, колесо баланса, полоса к цели.
// Разбирает только сервер: второго разбирателя того же формата в браузере нет.
package main

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Vault struct {
	Generated  time.Time
	Docs       map[string]*Doc // id → карточка
	Index      []*Doc          // в порядке обхода
	Tags       map[string]int
	Tree       *treeNode
	FolderMeta map[string]folderMeta
	Tasks      *TaskSet
	Metrics    *Metrics

	// разобранное для дашборда
	Vows    []Vow
	VowLog  *VowLog
	Diary   *DiaryEntry
	Spheres []Sphere
	Goal    *GoalBar
}

type TaskSet struct {
	Items []taskItem
	Open  int
	Done  int
}

type Metrics struct {
	Source  string
	Title   string
	Demo    bool
	Columns []string
	Rows    []metricRow
}

// ─────────────────── столпы ───────────────────

const (
	vowHeld  = "✓"
	vowBroke = "✗"
	vowsCard = "pillars/daily-vows"
)

type Vow struct {
	Name   string
	Rule   string
	Streak int    // дней подряд
	Record int    // самая длинная череда за всю историю
	Mark   string // отметка за сегодня: ✓, ✗ или пусто
}

// VowLog — таблица журнала отметок из той же карточки.
type VowLog struct {
	Header []string
	Rows   [][]string
}

var reVowLine = regexp.MustCompile(`^\s*[-*]\s+\*\*(.+?)\*\*\s*[—–-]?\s*(.*)$`)

// mdSection — тело раздела «## Заголовок» до следующего такого же.
func mdSection(body, title string) (string, bool) {
	for _, p := range splitH2(body) {
		head, rest, _ := strings.Cut(p, "\n")
		if strings.TrimSpace(head) == title {
			return rest, true
		}
	}
	return "", false
}

func tableCells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	out := strings.Split(line, "|")
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	return out
}

// isVowHeader — строка шапки журнала: таблица, первая ячейка «Дата».
func isVowHeader(line string) bool {
	if !strings.HasPrefix(strings.TrimSpace(line), "|") {
		return false
	}
	c := tableCells(line)
	return len(c) > 0 && c[0] == "Дата"
}

// parseVowLog — шапка и строки журнала отметок.
func parseVowLog(raw string) *VowLog {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	hi := -1
	for i, l := range lines {
		if isVowHeader(l) {
			hi = i
			break
		}
	}
	if hi < 0 {
		return nil
	}
	log := &VowLog{Header: tableCells(lines[hi])}
	for i := hi + 2; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|"); i++ {
		c := tableCells(lines[i])
		if len(c) > 0 && reISODate.MatchString(c[0]) {
			log.Rows = append(log.Rows, c)
		}
	}
	return log
}

// streakOf: идём назад по календарю от сегодня. Неотмеченный сегодняшний день
// цепь не рвёт — он ещё не кончился; пропущенный вчерашний рвёт.
func (l *VowLog) streakOf(name string, today time.Time) (cur, rec int, mark string) {
	col := -1
	for i, h := range l.Header {
		if i > 0 && h == name {
			col = i
			break
		}
	}
	if col < 1 {
		return 0, 0, ""
	}
	byDate := map[string]string{}
	for _, r := range l.Rows {
		if col < len(r) {
			byDate[r[0]] = r[col]
		}
	}
	day := today
	for k := 0; k < 3650; k++ {
		v := byDate[ymd(day)]
		if v == vowHeld {
			cur++
		} else if !(k == 0 && v == "") {
			break
		}
		day = day.AddDate(0, 0, -1)
	}
	held := []string{}
	for d, v := range byDate {
		if v == vowHeld {
			held = append(held, d)
		}
	}
	sort.Strings(held)
	run, prev := 0, ""
	for _, d := range held {
		if prev != "" && daysBetween(prev, d) == 1 {
			run++
		} else {
			run = 1
		}
		if run > rec {
			rec = run
		}
		prev = d
	}
	if cur > rec {
		rec = cur
	}
	return cur, rec, byDate[ymd(today)]
}

// marksOn — отметки за дату из журнала: с чего начинать правку строки.
func (l *VowLog) marksOn(date string) map[string]string {
	out := map[string]string{}
	for _, row := range l.Rows {
		if len(row) == 0 || row[0] != date {
			continue
		}
		for i, h := range l.Header {
			if i > 0 && i < len(row) {
				out[h] = row[i]
			}
		}
	}
	return out
}

// upsertVowRow — строка журнала за дату: правим существующую или дописываем
// в конец таблицы. Работает с сырым текстом карточки: источник истины —
// markdown, а не память сервера.
func upsertVowRow(raw string, header []string, date string, marks map[string]string) (string, bool) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	hi := -1
	for i, l := range lines {
		if isVowHeader(l) {
			hi = i
			break
		}
	}
	if hi < 0 {
		return raw, false
	}
	cells := []string{date}
	for _, h := range header[1:] {
		cells = append(cells, marks[h])
	}
	row := "| " + strings.Join(cells, " | ") + " |"

	end := hi + 2
	for end < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[end]), "|") {
		end++
	}
	for i := hi + 2; i < end; i++ {
		if c := tableCells(lines[i]); len(c) > 0 && c[0] == date {
			lines[i] = row
			return strings.Join(lines, "\n"), true
		}
	}
	out := append([]string{}, lines[:end]...)
	out = append(out, row)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n"), true
}

// ─────────────────── дневник ───────────────────

type DiaryEntry struct {
	When string
	Text string
}

const diaryCard = "notes/diary"

// дела на день: незакрытые чекбоксы раздела «Текущий день»
const (
	dayCard      = "tasks/day"
	todaySection = "## Текущий день"
	// заглушка пустого дня — та же, что в свежей карточке
	todayPlaceholder = "_Пусто. Назначь дела на сегодня._"
)

// lastDiaryEntry — последняя запись: заголовок «## дата [время]» и первый
// абзац под ним.
func lastDiaryEntry(body string) *DiaryEntry {
	all := diaryEntries(body)
	if len(all) == 0 {
		return nil
	}
	return &all[len(all)-1]
}

// ─────────────────── колесо баланса ───────────────────

type Sphere struct {
	Name  string
	Score int
	Max   int
	Pct   int
	Low   bool // ниже половины — провисает
	Goals []string
	Key   string // «здоровье» — как сферу зовут в `sphere:` карточек
	Cards []*Doc // цели, планы и проекты, которые отвечают этой сфере
}

// sphereKey — имя сферы без эмодзи и регистра: «💪 Здоровье» → «здоровье».
// Так её пишут в `sphere:` целей и планов (SOUL.md, «Хранилище»).
func sphereKey(name string) string {
	name = strings.TrimLeftFunc(name, func(r rune) bool { return !unicode.IsLetter(r) })
	return strings.ToLower(strings.TrimSpace(name))
}

// linkSpheres раскладывает по сферам карточки с `sphere:`. Архив не в счёт:
// закрытое сфере уже не отвечает.
func linkSpheres(v *Vault) {
	at := map[string]int{}
	for i := range v.Spheres {
		at[v.Spheres[i].Key] = i
	}
	for _, d := range v.Index {
		i, ok := at[strings.ToLower(fmString(d.FM, "sphere"))]
		if ok && d.Status != "archived" {
			v.Spheres[i].Cards = append(v.Spheres[i].Cards, d)
		}
	}
}

var (
	reSphereHead = regexp.MustCompile(`^(.+?)\s*\((\d+)\s*/\s*(\d+)\)$`)
	reGoalLine   = regexp.MustCompile(`^\s*[-*]\s+(.*)$`)
	reWikiPlain  = regexp.MustCompile(`\[\[([^\]|]+)(\|([^\]]+))?\]\]`)
	reISOPrefix  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)
)

const goalsCard = "pillars/life-goals"

// unlinkWiki — [[цель|подпись]] в обычный текст. В прежней реализации на JS
// шаблон замены был «$3$1», и подпись склеивалась с целью: «уставpillars/
// inviolable». Здесь берём подпись, а цель — только когда подписи нет.
func unlinkWiki(s string) string {
	return reWikiPlain.ReplaceAllStringFunc(s, func(m string) string {
		sub := reWikiPlain.FindStringSubmatch(m)
		if sub[3] != "" {
			return sub[3]
		}
		return sub[1]
	})
}

// parseSpheres — «## 💪 Здоровье (6/10)» и цели списком под заголовком.
// Порядок — от провисающей к сильной: вопрос к панели один, какая сфера
// просела, и ответ должен стоять первой строкой.
func parseSpheres(body string) []Sphere {
	out := []Sphere{}
	for _, p := range splitH2(body) {
		head, rest, _ := strings.Cut(p, "\n")
		m := reSphereHead.FindStringSubmatch(strings.TrimSpace(head))
		if m == nil {
			continue
		}
		score, _ := strconv.Atoi(m[2])
		max, _ := strconv.Atoi(m[3])
		if max == 0 {
			max = 10
		}
		goals := []string{}
		for _, l := range strings.Split(rest, "\n") {
			if g := reGoalLine.FindStringSubmatch(strings.TrimSuffix(l, "\r")); g != nil {
				goals = append(goals, strings.TrimSpace(unlinkWiki(g[1])))
			}
		}
		out = append(out, Sphere{
			Name: strings.TrimSpace(m[1]), Key: sphereKey(m[1]), Score: score, Max: max,
			Pct: clampInt(int(math.Round(float64(score)/float64(max)*100)), 0, 100),
			Low: score*2 < max, Goals: goals,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score < out[j].Score })
	return out
}

// ─────────────────── полоса к цели ───────────────────

type GoalBar struct {
	Now    float64
	Target float64
	Start  float64
	Left   float64
	Pct    int
	Rate   float64 // кг в неделю
	Title  string
	Source string
}

// buildGoalBar — «старт → сейчас → цель». Цель берётся из любой карточки
// с target_kg во frontmatter: график показывает движение, но не отвечает,
// сколько осталось.
func buildGoalBar(v *Vault) *GoalBar {
	if v.Metrics == nil || len(v.Metrics.Rows) < 2 {
		return nil
	}
	var goal *Doc
	for _, d := range v.Index {
		if fmString(d.FM, "target_kg") != "" {
			goal = d
			break
		}
	}
	if goal == nil {
		return nil
	}
	target, err := strconv.ParseFloat(strings.ReplaceAll(fmString(goal.FM, "target_kg"), ",", "."), 64)
	if err != nil {
		return nil
	}
	first, last := v.Metrics.Rows[0], v.Metrics.Rows[len(v.Metrics.Rows)-1]
	if len(first.Values) == 0 || len(last.Values) == 0 || first.Values[0] == nil || last.Values[0] == nil {
		return nil
	}
	start, now := *first.Values[0], *last.Values[0]
	if start == target {
		return nil
	}
	rate := 0.0
	if days := float64(daysBetween(first.Date, last.Date)); days > 0 {
		rate = (start - now) / days * 7
	}
	return &GoalBar{
		Now: now, Target: target, Start: start,
		Left:   math.Round((now-target)*10) / 10,
		Pct:    clampInt(int(math.Round((start-now)/(start-target)*100)), 0, 100),
		Rate:   rate,
		Title:  goal.Title,
		Source: goal.ID,
	}
}

// ─────────────────── сборка дашборда ───────────────────

func buildDashboard(v *Vault) {
	if d := v.Docs[vowsCard]; d != nil {
		if sec, ok := mdSection(d.Body, "Столпы"); ok {
			v.VowLog = parseVowLog(d.Body)
			today := time.Now()
			for _, l := range strings.Split(sec, "\n") {
				m := reVowLine.FindStringSubmatch(strings.TrimSuffix(l, "\r"))
				if m == nil {
					continue
				}
				vow := Vow{Name: strings.TrimSpace(m[1]), Rule: strings.TrimSpace(m[2])}
				if v.VowLog != nil {
					vow.Streak, vow.Record, vow.Mark = v.VowLog.streakOf(vow.Name, today)
				}
				v.Vows = append(v.Vows, vow)
			}
		}
	}
	if d := v.Docs[diaryCard]; d != nil {
		v.Diary = lastDiaryEntry(d.Body)
	}
	if d := v.Docs[goalsCard]; d != nil {
		v.Spheres = parseSpheres(d.Body)
		linkSpheres(v)
	}
	v.Goal = buildGoalBar(v)
}

// ─────────────────── мелочи ───────────────────

// splitH2 — куски карточки по заголовкам второго уровня.
func splitH2(body string) []string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	if !strings.HasPrefix(body, "## ") {
		i := strings.Index(body, "\n## ")
		if i < 0 {
			return nil
		}
		body = body[i+1:]
	}
	return strings.Split(strings.TrimPrefix(body, "## "), "\n## ")
}

func ymd(t time.Time) string { return t.Format("2006-01-02") }

// daysBetween — сколько суток между двумя датами вида ГГГГ-ММ-ДД.
func daysBetween(a, b string) int {
	ta, err1 := time.Parse("2006-01-02", a)
	tb, err2 := time.Parse("2006-01-02", b)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(math.Round(tb.Sub(ta).Hours() / 24))
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
