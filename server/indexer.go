// Индексатор хранилища: сканирует .md, разбирает frontmatter, резолвит
// [[вики-ссылки]] и обратные ссылки, строит дерево папок и данные дашборда. Результат — модель в памяти (vault.go), по которой
// сервер рисует страницы. В браузер она целиком не уходит.
//
// Перенесён из viewer/build.mjs, чтобы на проде не требовался Node.
package main

import (
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// каталоги с кодом и служебным, а не карточками: единственный список на всё
// хранилище — им пользуются сборка модели, линтер, сторож и запрет правки.
// .claude — рабочие копии агента (worktrees): без него каждая карточка
// находилась поиском дважды, а линтер ругался на чужие черновики.
var ignoreDirs = map[string]bool{
	".git": true, ".obsidian": true, ".claude": true, "node_modules": true, "vendor": true,
	"viewer": true, "deploy": true, "server": true, ".github": true, "android": true, "web": true,
}

type folderMeta struct {
	Label string
	Icon  string
	Desc  string
}

// ярлык, иконка и описание раздела — для сайдбара и карточек разделов
var folderMetaMap = map[string]folderMeta{
	".":          {Label: "Корень", Icon: "🏛️"},
	"_sys":       {Label: "Система", Icon: "⚙️", Desc: "Шаблоны, конфиги, скрипты"},
	"inbox":      {Label: "Входящие", Icon: "📥", Desc: "Сырьё до сортировки"},
	"business":   {Label: "Бизнес", Icon: "💼", Desc: "Проекты хозяина"},
	"tasks":      {Label: "Задачи", Icon: "✅", Desc: "Дела, бэклог"},
	"concepts":   {Label: "Концепции", Icon: "💡", Desc: "Живые идеи, проекты"},
	"pillars":    {Label: "Столпы", Icon: "🏛️", Desc: "Фундаментальные принципы"},
	"facts":      {Label: "Факты", Icon: "🔬", Desc: "Сухие данные, нутрицевтики"},
	"guides":     {Label: "Гайды", Icon: "📖", Desc: "Инструкции, чеклисты"},
	"hypotheses": {Label: "Гипотезы", Icon: "🧪", Desc: "Предположения"},
	"notes":      {Label: "Заметки", Icon: "📝", Desc: "Обработанные записи"},
	"goals":      {Label: "Цели", Icon: "🎯", Desc: "Целеполагание"},
	"plans":      {Label: "Планы", Icon: "🗓️", Desc: "Программы и протоколы"},
	"archive":    {Label: "Архив", Icon: "🗄️", Desc: "Закрытые вопросы"},
}

var (
	reFrontmatter = regexp.MustCompile(`(?s)\A---\r?\n(.*?)\r?\n---\r?\n?`)
	reFencedCode  = regexp.MustCompile("(?s)```.*?```")
	reInlineCode  = regexp.MustCompile("`[^`\n]*`")
	reWikilink    = regexp.MustCompile(`!?\[\[([^\]]+?)\]\]`)
	reHeading1    = regexp.MustCompile(`(?m)^#\s+(.+)$`)
	reListID      = regexp.MustCompile(`^-?\s*id:\s*(.+)$`)
	reTaskLine    = regexp.MustCompile(`^\s*[-*]\s+\[([ xX])\]\s+(.+?)\s*$`)
	reTaskGroup   = regexp.MustCompile(`^#{2,3}\s+(.+?)\s*$`)
	reTableRule   = regexp.MustCompile(`^:?-{2,}:?$`)
	reISODate     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	reSpaces      = regexp.MustCompile(`\s+`)
	reTagSplit    = regexp.MustCompile(`[, ]+`)
)

type linkRef struct {
	ID      string
	Display string
}

// Doc — карточка хранилища в том виде, в каком её видят шаблоны.
// Поля экспортированы намеренно: страницы рисует сервер, и html/template
// читает только то, что вынесено наружу.
type Doc struct {
	ID        string
	Path      string
	Title     string
	Folder    string
	TopFolder string
	Type      string
	Status    string
	Summary   string
	Created   string
	Tags      []string
	FM        map[string]any
	Body      string
	Plain     string // тело без разметки — для поиска и выдержек
	Wordcount int
	Links     []linkRef
	Backlinks []string
}

// ─────────────────── frontmatter ───────────────────

// parseFrontmatter разбирает подмножество YAML: скаляры, инлайн-массивы
// [a, b] и блочные списки объектов, из которых берётся только id (edges).
// Возвращает nil, если frontmatter отсутствует.
func parseFrontmatter(text string) (map[string]any, string) {
	m := reFrontmatter.FindStringSubmatchIndex(text)
	if m == nil {
		return nil, text
	}
	head := text[m[2]:m[3]]
	body := text[m[1]:]
	data := map[string]any{}
	listKey := ""
	for _, raw := range strings.Split(head, "\n") {
		raw = strings.TrimSuffix(raw, "\r")
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if listKey != "" && (strings.HasPrefix(raw, " ") || strings.HasPrefix(raw, "\t")) {
			if im := reListID.FindStringSubmatch(line); im != nil {
				if arr, ok := data[listKey].([]string); ok {
					data[listKey] = append(arr, strings.TrimSpace(im[1]))
				}
			}
			continue
		}
		idx := strings.Index(line, ":")
		if idx == -1 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		listKey = ""
		switch {
		case val == "":
			data[key] = []string{}
			listKey = key
		case strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]"):
			items := []string{}
			for _, p := range strings.Split(val[1:len(val)-1], ",") {
				if s := trimQuotes(strings.TrimSpace(p)); s != "" {
					items = append(items, s)
				}
			}
			data[key] = items
		default:
			data[key] = trimQuotes(val)
		}
	}
	return data, body
}

func trimQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return strings.Trim(s, `"'`)
}

func fmString(fm map[string]any, key string) string {
	if v, ok := fm[key].(string); ok {
		return v
	}
	return ""
}

// ─────────────────── вики-ссылки ───────────────────

// extractWikilinks: [[цель|подпись]] и ![[встройка]]. Примеры внутри
// код-блоков ссылками не считаются.
func extractWikilinks(body string) []linkRef {
	clean := reInlineCode.ReplaceAllString(reFencedCode.ReplaceAllString(body, ""), "")
	out := []linkRef{}
	for _, m := range reWikilink.FindAllStringSubmatch(clean, -1) {
		inner := m[1]
		inner = strings.ReplaceAll(inner, `\|`, "|")
		parts := strings.Split(inner, "|")
		target := strings.TrimSpace(strings.Split(parts[0], "#")[0])
		target = strings.TrimSuffix(target, ".md")
		target = strings.TrimSuffix(target, "/")
		if target == "" {
			continue
		}
		display := strings.TrimSpace(parts[0])
		if len(parts) > 1 {
			display = strings.TrimSpace(parts[1])
		}
		out = append(out, linkRef{ID: target, Display: display})
	}
	return out
}

// ─────────────────── сборка ───────────────────

// buildIndex читает хранилище и собирает модель, по которой сервер рисует
// страницы. Наружу без сессии не уходит ни строки из неё.
func buildIndex(vaultRoot string) (*Vault, error) {
	files, err := walkMarkdown(vaultRoot)
	if err != nil {
		return nil, err
	}

	docs := make([]*Doc, 0, len(files))
	byID := map[string]*Doc{}
	byBasename := map[string][]string{}

	for _, full := range files {
		raw, err := os.ReadFile(full)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(vaultRoot, full)
		if err != nil {
			return nil, err
		}
		relSlash := filepath.ToSlash(rel)
		id := strings.TrimSuffix(relSlash, filepath.Ext(relSlash))
		if !strings.EqualFold(filepath.Ext(relSlash), ".md") {
			continue
		}

		fm, body := parseFrontmatter(string(raw))
		if fm == nil {
			fm = map[string]any{}
		}

		title := fmString(fm, "title")
		if title == "" {
			if h := reHeading1.FindStringSubmatch(body); h != nil {
				title = strings.TrimSpace(h[1])
			}
		}
		if title == "" {
			title = strings.TrimSuffix(path.Base(relSlash), ".md")
		}

		folder := path.Dir(relSlash)
		topFolder := folder
		if folder != "." {
			topFolder = strings.Split(folder, "/")[0]
		}

		tags := []string{}
		switch v := fm["tags"].(type) {
		case []string:
			tags = v
		case string:
			tags = splitFields(v)
		default:
			switch v2 := fm["tag"].(type) {
			case []string:
				tags = v2
			case string:
				tags = splitFields(v2)
			}
		}

		d := &Doc{
			ID: id, Path: relSlash, Title: title, Folder: folder, TopFolder: topFolder,
			Type: fmString(fm, "type"), Status: fmString(fm, "status"),
			Summary: fmString(fm, "summary"), Created: fmString(fm, "created"),
			Tags: tags, FM: fm, Body: body, Plain: plainText(body), Wordcount: countWords(body),
			Links: []linkRef{}, Backlinks: []string{},
		}
		docs = append(docs, d)
		byID[id] = d
		bn := strings.ToLower(path.Base(id))
		byBasename[bn] = append(byBasename[bn], id)
	}

	// резолв ссылок и обратных ссылок
	for _, d := range docs {
		seen := map[string]bool{}
		for _, l := range extractWikilinks(d.Body) {
			resolved := resolveTarget(l.ID, byID, byBasename)
			if resolved == "" || resolved == d.ID || seen[resolved] {
				continue
			}
			seen[resolved] = true
			d.Links = append(d.Links, linkRef{ID: resolved, Display: l.Display})
		}
	}
	for _, d := range docs {
		for _, l := range d.Links {
			if tgt, ok := byID[l.ID]; ok && !contains(tgt.Backlinks, d.ID) {
				tgt.Backlinks = append(tgt.Backlinks, d.ID)
			}
		}
	}

	// теги и дерево
	tagCounts := map[string]int{}
	for _, d := range docs {
		for _, t := range d.Tags {
			tagCounts[t]++
		}
	}

	v := &Vault{
		Generated:  time.Now(),
		Docs:       byID,
		Index:      docs,
		Tags:       tagCounts,
		Tree:       buildTree(docs),
		FolderMeta: folderMetaMap,
		Tasks:      collectTasks(docs),
		Metrics:    collectMetrics(docs),
	}
	buildDashboard(v) // столпы, дневник, колесо — разбор живёт на сервере
	return v, nil
}

// resolveTarget: точное совпадение id, иначе по имени файла — из кандидатов
// берётся тот, чей путь совпадает с целью большим числом сегментов.
func resolveTarget(target string, byID map[string]*Doc, byBasename map[string][]string) string {
	if target == "" {
		return ""
	}
	t := strings.TrimPrefix(target, "./")
	if _, ok := byID[t]; ok {
		return t
	}
	cands := byBasename[strings.ToLower(path.Base(t))]
	if len(cands) == 0 {
		return "" // битая ссылка
	}
	segs := strings.Split(strings.ToLower(t), "/")
	best, bestScore := cands[0], -1
	for _, c := range cands {
		cl := strings.ToLower(c)
		score := 0
		for _, s := range segs {
			if strings.Contains(cl, s) {
				score++
			}
		}
		if score > bestScore {
			bestScore, best = score, c
		}
	}
	return best
}

type treeNode struct {
	Name  string
	Path  string
	Dirs  map[string]*treeNode
	Files []string
}

func buildTree(docs []*Doc) *treeNode {
	root := &treeNode{Name: ".", Path: ".", Dirs: map[string]*treeNode{}, Files: []string{}}
	for _, d := range docs {
		node := root
		if d.Folder != "." {
			acc := []string{}
			for _, part := range strings.Split(d.Folder, "/") {
				acc = append(acc, part)
				if node.Dirs[part] == nil {
					node.Dirs[part] = &treeNode{Name: part, Path: strings.Join(acc, "/"),
						Dirs: map[string]*treeNode{}, Files: []string{}}
				}
				node = node.Dirs[part]
			}
		}
		node.Files = append(node.Files, d.ID)
	}
	return root
}

// ─────────────────── дашборд ───────────────────

type taskItem struct {
	Text  string
	Done  bool
	Group string
	Src   string
	Due   string
}

// Дела живут в карточках tasks/ обычными markdown-чекбоксами; заголовок ##
// над строкой считается группой (приоритетом). Код-блоки вырезаются: там
// примеры формата, а не живые дела.
//
// Срок — хвост «@ГГГГ-ММ-ДД» в конце строки: он не ломает уже написанные дела
// и читается в самой карточке, а не только на дашборде.
func collectTasks(docs []*Doc) *TaskSet {
	items := []taskItem{}
	open, done := 0, 0
	for _, d := range docs {
		if d.TopFolder != "tasks" {
			continue
		}
		group := ""
		clean := reFencedCode.ReplaceAllString(d.Body, "")
		for _, line := range strings.Split(clean, "\n") {
			line = strings.TrimSuffix(line, "\r")
			if h := reTaskGroup.FindStringSubmatch(line); h != nil {
				group = strings.TrimSpace(h[1])
				continue
			}
			text, due, isDone, ok := parseTaskLine(line)
			if !ok {
				continue
			}
			if isDone {
				done++
			} else {
				open++
			}
			items = append(items, taskItem{Text: text, Done: isDone, Group: group, Src: d.ID, Due: due})
		}
	}
	if len(items) == 0 {
		return nil
	}
	return &TaskSet{Items: items, Open: open, Done: done}
}

// parseTaskLine разбирает строку-чекбокс: текст без жирного и хвоста срока,
// срок и отметку. Одна функция на сборку модели и на правку карточек —
// иначе дашборд и запись разошлись бы в том, что считать «тем же делом».
func parseTaskLine(line string) (text, due string, done, ok bool) {
	m := reTaskLine.FindStringSubmatch(strings.TrimSuffix(line, "\r"))
	if m == nil {
		return "", "", false, false
	}
	text = strings.TrimSpace(strings.ReplaceAll(m[2], "**", ""))
	if i := strings.LastIndex(text, "@"); i != -1 {
		if tail := text[i+1:]; reISODate.MatchString(tail) {
			due = tail
			text = strings.TrimSpace(text[:i])
		}
	}
	return text, due, strings.EqualFold(m[1], "x"), text != ""
}

type metricRow struct {
	Date   string
	Values []*float64
}

// Замеры тела: карточка с `metrics: body` во frontmatter, первая
// markdown-таблица — первая колонка дата, остальные числовые ряды.
func collectMetrics(docs []*Doc) *Metrics {
	var card *Doc
	for _, d := range docs {
		if fmString(d.FM, "metrics") == "body" {
			card = d
			break
		}
	}
	if card == nil {
		return nil
	}
	var columns []string
	rows := []metricRow{}
	for _, line := range strings.Split(card.Body, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if !strings.HasPrefix(strings.TrimLeft(line, " \t"), "|") {
			if columns != nil && len(rows) > 0 {
				break
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimPrefix(trimmed, "|")
		trimmed = strings.TrimSuffix(trimmed, "|")
		cells := []string{}
		for _, c := range strings.Split(trimmed, "|") {
			cells = append(cells, strings.TrimSpace(c))
		}
		allRule := true
		for _, c := range cells {
			if !reTableRule.MatchString(c) {
				allRule = false
				break
			}
		}
		if allRule {
			continue // разделитель шапки
		}
		if columns == nil {
			columns = cells
			continue
		}
		if len(cells) == 0 || !reISODate.MatchString(cells[0]) {
			continue
		}
		values := make([]*float64, 0, len(cells)-1)
		for _, v := range cells[1:] {
			n, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
				values = append(values, nil)
				continue
			}
			val := n
			values = append(values, &val)
		}
		rows = append(rows, metricRow{Date: cells[0], Values: values})
	}
	if columns == nil { // таблицы нет — панели нет; одна строка — панель с точкой и формой ввода
		return nil
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Date < rows[j].Date })
	return &Metrics{
		Source:  card.ID,
		Title:   card.Title,
		Demo:    fmString(card.FM, "demo") == "true",
		Columns: columns,
		Rows:    rows,
	}
}

// ─────────────────── мелочи ───────────────────

// walkMarkdown обходит хранилище в одном порядке для всех, кто его читает:
// имена в каталоге по алфавиту, в подкаталог заходим на его месте.
// Порядок важен: от него зависит порядок ссылок и обратных ссылок.
func walkMarkdown(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	out := []string{}
	for _, e := range entries {
		full := filepath.Join(dir, e.Name())
		if e.IsDir() {
			if ignoreDirs[e.Name()] {
				continue
			}
			nested, err := walkMarkdown(full)
			if err != nil {
				return nil, err
			}
			out = append(out, nested...)
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			out = append(out, full)
		}
	}
	return out, nil
}

func countWords(body string) int {
	n := 0
	inWord := false
	for _, r := range strings.TrimSpace(body) {
		if unicode.IsSpace(r) {
			inWord = false
			continue
		}
		if !inWord {
			n++
			inWord = true
		}
	}
	return n
}

func splitFields(s string) []string {
	out := []string{}
	for _, p := range reTagSplit.Split(s, -1) {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func normalizeSpaces(s string) string { return reSpaces.ReplaceAllString(s, " ") }
