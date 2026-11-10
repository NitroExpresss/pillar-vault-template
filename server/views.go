// Данные экранов: дерево меню, темы, сроки, дела дня, колесо, подписи дат.
// Раньше их звали шаблоны прежнего интерфейса, теперь — API приложения
// (api.go, board.go, screens.go, widget.go): разбор один, сколько бы ни было
// тех, кто рисует.
package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ─────────────────── помощники ───────────────────

func plural(n int, one, few, many string) string {
	a := n % 100
	if a < 0 {
		a = -a
	}
	b := a % 10
	switch {
	case a > 10 && a < 20:
		return many
	case b == 1:
		return one
	case b >= 2 && b <= 4:
		return few
	}
	return many
}

// dueLabel — срок человеческим языком: «вчера», «через 3 дня», иначе дата.
func dueLabel(due, today string) string {
	days := daysBetween(today, due)
	switch {
	case days == 0:
		return "сегодня"
	case days == 1:
		return "завтра"
	case days == -1:
		return "вчера"
	case days < 0:
		return fmt.Sprintf("%d %s назад", -days, plural(-days, "день", "дня", "дней"))
	case days <= 14:
		return fmt.Sprintf("через %d %s", days, plural(days, "день", "дня", "дней"))
	}
	return shortDate(due)
}

func dueClass(due, today string) string {
	switch {
	case due < today:
		return "over"
	case due == today:
		return "now"
	}
	return "next"
}

func snippet(d *Doc) string {
	if d.Summary != "" {
		return d.Summary
	}
	r := []rune(d.Plain)
	if len(r) <= 160 {
		return d.Plain
	}
	return wordWindow(r, 0, 160)
}

// cutRunes режет по символам, а не по байтам: в кириллице байтовая обрезка
// разваливает последнюю букву и вместе с ней — кодировку всей страницы.
func cutRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func num(f float64) string {
	return strconv.FormatFloat(math.Round(f*10)/10, 'f', -1, 64)
}

// dateRu — «9 сентября, среда»: месяц и день недели по-русски, потому что
// локалей в стандартной библиотеке Go нет и не будет.
var (
	ruMonths = [...]string{"января", "февраля", "марта", "апреля", "мая", "июня",
		"июля", "августа", "сентября", "октября", "ноября", "декабря"}
	ruDays = map[time.Weekday]string{
		time.Monday: "понедельник", time.Tuesday: "вторник", time.Wednesday: "среда",
		time.Thursday: "четверг", time.Friday: "пятница", time.Saturday: "суббота",
		time.Sunday: "воскресенье"}
)

func dateRu(day string) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	return fmt.Sprintf("%d %s, %s", t.Day(), ruMonths[t.Month()-1], ruDays[t.Weekday()])
}

// ─────────────────── дела ───────────────────

type taskGroup struct {
	Name  string
	Items []taskItem
}

// groupTasks — дела по подзаголовкам карточки, в порядке их появления.
func groupTasks(t *TaskSet) []taskGroup {
	if t == nil {
		return nil
	}
	order := []string{}
	byName := map[string][]taskItem{}
	for _, i := range t.Items {
		if _, ok := byName[i.Group]; !ok {
			order = append(order, i.Group)
		}
		byName[i.Group] = append(byName[i.Group], i)
	}
	out := make([]taskGroup, 0, len(order))
	for _, name := range order {
		out = append(out, taskGroup{Name: name, Items: byName[name]})
	}
	return out
}

func donePct(t *TaskSet) int {
	total := t.Open + t.Done
	if total == 0 {
		return 0
	}
	return int(math.Round(float64(t.Done) / float64(total) * 100))
}

// tasksOf — всё, кроме дня и покупок: у них свои панели.
func tasksOf(v *Vault) *TaskSet {
	if v.Tasks == nil {
		return nil
	}
	items := []taskItem{}
	open, done := 0, 0
	for _, i := range v.Tasks.Items {
		if i.Src == dayCard || i.Src == shoppingCard { // день и покупки — своими блоками
			continue
		}
		items = append(items, i)
		if i.Done {
			done++
		} else {
			open++
		}
	}
	if len(items) == 0 {
		return nil
	}
	return &TaskSet{Items: items, Open: open, Done: done}
}

func todayList(v *Vault) []taskItem {
	out := []taskItem{}
	if v.Tasks == nil {
		return out
	}
	for _, i := range v.Tasks.Items {
		if i.Src == dayCard && !i.Done {
			out = append(out, i)
		}
	}
	return out
}

// pickableTasks — открытые дела из всех карточек, кроме самого дня и того,
// что на сегодня уже стоит: список, из которого день пополняется.
func pickableTasks(v *Vault) []taskGroup {
	if v.Tasks == nil {
		return nil
	}
	onToday := map[string]bool{}
	for _, i := range todayList(v) {
		onToday[i.Text] = true
	}
	items := []taskItem{}
	for _, i := range v.Tasks.Items {
		if !i.Done && i.Src != dayCard && i.Src != shoppingCard && !onToday[i.Text] {
			items = append(items, i)
		}
	}
	if len(items) == 0 {
		return nil
	}
	return groupTasks(&TaskSet{Items: items})
}

type burnZone struct {
	Key   string
	Title string
	Items []taskItem
}

// burnZones — просроченное, сегодняшнее и будущее: три полосы, а не один
// список. Срок вчера и срок через месяц требуют разных действий.
func burnZones(v *Vault, today string) []burnZone {
	if v.Tasks == nil {
		return nil
	}
	due := []taskItem{}
	for _, i := range v.Tasks.Items {
		if !i.Done && i.Due != "" {
			due = append(due, i)
		}
	}
	if len(due) == 0 {
		return nil
	}
	sort.SliceStable(due, func(a, b int) bool { return due[a].Due < due[b].Due })
	zones := []burnZone{{Key: "over", Title: "Просрочено"}, {Key: "now", Title: "Сегодня"}, {Key: "next", Title: "Дальше"}}
	for _, i := range due {
		switch dueClass(i.Due, today) {
		case "over":
			zones[0].Items = append(zones[0].Items, i)
		case "now":
			zones[1].Items = append(zones[1].Items, i)
		default:
			zones[2].Items = append(zones[2].Items, i)
		}
	}
	out := []burnZone{}
	for _, z := range zones {
		if len(z.Items) > 0 {
			out = append(out, z)
		}
	}
	return out
}

// ─────────────────── карточка ───────────────────

func linksOf(v *Vault, d *Doc) []*Doc {
	out := []*Doc{}
	for _, l := range d.Links {
		if t := v.Docs[l.ID]; t != nil {
			out = append(out, t)
		}
	}
	return out
}

func backlinksOf(v *Vault, d *Doc) []*Doc {
	out := []*Doc{}
	for _, id := range d.Backlinks {
		if t := v.Docs[id]; t != nil {
			out = append(out, t)
		}
	}
	return out
}

// ─────────────────── сайдбар ───────────────────

type treeView struct {
	Dirs  []treeDirView
	Files []treeFileView
}

type treeDirView struct {
	Node      *treeNode
	Icon      string
	Label     string
	Count     int
	Collapsed bool
	Active    bool // открыт этот раздел
	Child     treeView
}

type treeFileView struct {
	Doc    *Doc
	Active bool // открыта эта карточка
}

// порядок разделов в дереве: смысловой, а не алфавитный
var folderOrder = []string{"pillars", "concepts", "business", "plans", "facts",
	"guides", "hypotheses", "notes", "tasks", "goals", "inbox", "archive", "_sys"}

// treeRoot — дерево сайдбара с отметкой, где ты сейчас: открытая карточка
// или раздел подсвечены, а ветки до них раскрыты — иначе с телефона не
// видно, в каком месте хранилища находишься.
func treeRoot(v *Vault, activeDoc, activeDir string) treeView {
	here := activeDir
	if d := v.Docs[activeDoc]; d != nil {
		here = d.Folder
	}
	return treeLevel(v, v.Tree, 0, activeDoc, activeDir, here)
}

func treeLevel(v *Vault, node *treeNode, depth int, activeDoc, activeDir, here string) treeView {
	out := treeView{}
	if node == nil {
		return out
	}
	names := make([]string, 0, len(node.Dirs))
	for name := range node.Dirs {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		ii, ji := indexOf(folderOrder, names[i]), indexOf(folderOrder, names[j])
		if ii != ji {
			return ii < ji
		}
		return names[i] < names[j]
	})
	for _, name := range names {
		dir := node.Dirs[name]
		meta := v.FolderMeta[name]
		icon, label := meta.Icon, meta.Label
		if icon == "" {
			icon = "📁"
		}
		if label == "" {
			label = name
		}
		onPath := here == dir.Path || strings.HasPrefix(here, dir.Path+"/")
		out.Dirs = append(out.Dirs, treeDirView{
			Node: dir, Icon: icon, Label: label, Count: countFiles(dir),
			Collapsed: depth >= 1 && !onPath, Active: activeDir == dir.Path,
			Child: treeLevel(v, dir, depth+1, activeDoc, activeDir, here),
		})
	}
	files := []*Doc{}
	for _, id := range node.Files {
		if d := v.Docs[id]; d != nil {
			files = append(files, d)
		}
	}
	for _, d := range sortByTitle(files) {
		out.Files = append(out.Files, treeFileView{Doc: d, Active: d.ID == activeDoc})
	}
	return out
}

func countFiles(node *treeNode) int {
	n := len(node.Files)
	for _, d := range node.Dirs {
		n += countFiles(d)
	}
	return n
}

func indexOf(list []string, s string) int {
	for i, x := range list {
		if x == s {
			return i
		}
	}
	return 99
}

type tagView struct {
	Name  string
	Count int
	Share int
	Kids  []tagKid
}

type tagKid struct {
	Name  string
	Short string
	Count int
}

// topTags — только темы с ветками или частотой ≥ 2: редкие ищутся поиском.
func topTags(v *Vault) []tagView {
	tops := map[string]*tagView{}
	for t, n := range v.Tags {
		top := strings.Split(t, "/")[0]
		if tops[top] == nil {
			tops[top] = &tagView{Name: top}
		}
		tops[top].Count += n
		if strings.Contains(t, "/") {
			tops[top].Kids = append(tops[top].Kids, tagKid{Name: t, Short: t[len(top)+1:], Count: n})
		}
	}
	out := []tagView{}
	max := 0
	for _, tv := range tops {
		if tv.Count < 2 && len(tv.Kids) == 0 {
			continue
		}
		if tv.Count > max {
			max = tv.Count
		}
		sort.Slice(tv.Kids, func(i, j int) bool { return tv.Kids[i].Name < tv.Kids[j].Name })
		out = append(out, *tv)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	for i := range out {
		if max > 0 {
			out[i].Share = out[i].Count * 100 / max
		}
	}
	return out
}
