// Новая карточка из приложения — без агента. Форма даёт то, что устав
// требует от любой карточки (SOUL.md, «Хранилище» и «Теги»): тип, summary,
// первый тег из дерева, свободные теги по формату. Frontmatter собирает
// сервер — руками его не испортить, и линтер такую карточку пропускает.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// cardTypes — типы карточек из устава, по-русски и с разделом по умолчанию.
var cardTypes = []struct {
	Value  string `json:"value"`
	Label  string `json:"label"`
	Folder string `json:"folder,omitempty"`
}{
	{"note", "заметка", "notes"},
	{"fact", "факт", "facts"},
	{"supplement", "добавка", "facts"},
	{"guide", "гайд", "guides"},
	{"hypothesis", "гипотеза", "hypotheses"},
	{"concept", "концепция", "concepts"},
	{"plan", "план", "plans"},
	{"goal", "цель", "goals"},
	{"task", "дела", "tasks"},
	{"question", "вопрос", "notes"},
	{"source", "источник", "notes"},
	{"area", "сфера", ""},
	{"pillar", "столп", "pillars"},
	{"archive", "архив", "archive"},
}

type newFolder struct {
	Path  string `json:"path"`
	Label string `json:"label"`
}

// newFolders — куда можно положить карточку: разделы хранилища (и пустые,
// если папка есть на диске) и их подпапки с карточками; служебное — нет.
func newFolders(v *Vault) []newFolder {
	out := []newFolder{}
	var walk func(n *treeNode)
	walk = func(n *treeNode) {
		names := make([]string, 0, len(n.Dirs))
		for name := range n.Dirs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			c := n.Dirs[name]
			out = append(out, newFolder{Path: c.Path, Label: folderLabel(v, c.Path)})
			walk(c)
		}
	}
	for _, top := range folderOrder {
		if top == "_sys" {
			continue
		}
		if st, err := os.Stat(filepath.Join(root, top)); err != nil || !st.IsDir() {
			continue
		}
		out = append(out, newFolder{Path: top, Label: folderLabel(v, top)})
		if v.Tree != nil {
			if n := v.Tree.Dirs[top]; n != nil {
				walk(n)
			}
		}
	}
	return out
}

func taxonomyList() []string {
	out := make([]string, 0, len(taxonomy))
	for t := range taxonomy {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// handleNewAPI — GET /api/new: справочники формы; POST /api/new: создать.
func handleNewAPI(w http.ResponseWriter, r *http.Request) {
	if !widgetAuth(w, r) {
		return
	}
	v := currentVault()
	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(struct {
			Folders  []newFolder `json:"folders"`
			Types    any         `json:"types"`
			Taxonomy []string    `json:"taxonomy"`
		}{newFolders(v), cardTypes, taxonomyList()})
		return
	}
	if !wantsAPI(r) { // формы с чужого сайта без CORS-заголовка сюда не дойдут
		w.WriteHeader(http.StatusForbidden)
		return
	}
	r.ParseForm()
	nc := newCard{
		Title: r.FormValue("title"), Folder: r.FormValue("folder"), Type: r.FormValue("type"),
		Summary: r.FormValue("summary"), Tag: r.FormValue("tag"), Tags: r.FormValue("tags"),
		Deadline: r.FormValue("deadline"), Body: r.FormValue("body"),
		Inbox: r.FormValue("to") == "inbox",
	}
	id, err := createCard(v, nc, time.Now())
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(apiResult{Message: err.Error()})
		return
	}
	json.NewEncoder(w).Encode(struct {
		OK  bool   `json:"ok"`
		ID  string `json:"id"`
		URL string `json:"url"`
	}{true, id, docURL(id)})
}

type newCard struct {
	Title, Folder, Type, Summary, Tag, Tags, Deadline, Body string
	// Inbox — черновик во входящие: нужны только заголовок и текст, остальное
	// дооформит агент при разборе (кнопка «Разобрать с ИИ» во «Входящих»)
	Inbox bool
}

// createCard проверяет форму, собирает карточку и пишет её на диск.
func createCard(v *Vault, nc newCard, now time.Time) (string, error) {
	one := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	if nc.Inbox {
		nc.Folder = "inbox"
		if nc.Type == "" {
			nc.Type = "note"
		}
		if one(nc.Summary) == "" {
			nc.Summary = "Черновик во входящих — ждёт разбора: " + one(nc.Title)
		}
		if !taxonomy[nc.Tag] {
			nc.Tag = "pkm" // временно: при разборе агент поставит тему по смыслу
		}
	}
	title, summary := one(nc.Title), one(nc.Summary)
	switch {
	case title == "":
		return "", errors.New("нужен заголовок")
	case utf8.RuneCountInString(title) > 120:
		return "", errors.New("заголовок длиннее 120 символов")
	case summary == "":
		return "", errors.New("нужен summary — по нему карточку находят, не открывая")
	case utf8.RuneCountInString(summary) > 300:
		return "", errors.New("summary длиннее 300 символов — это уже текст карточки")
	}
	typ := ""
	for _, t := range cardTypes {
		if t.Value == nc.Type {
			typ = t.Value
		}
	}
	if typ == "" {
		return "", errors.New("нет такого типа")
	}
	if !taxonomy[nc.Tag] {
		return "", errors.New("первый тег — из дерева устава")
	}
	tags := []string{nc.Tag}
	for _, t := range strings.FieldsFunc(strings.ToLower(nc.Tags), func(r rune) bool { return r == ',' || r == ';' }) {
		t = strings.Join(strings.Fields(t), "-")
		switch {
		case t == "" || t == nc.Tag:
			continue
		case taxonomy[t]:
			return "", fmt.Errorf("«%s» — из дерева; таксономический тег у карточки один", t)
		case !reFreeTag.MatchString(t):
			return "", fmt.Errorf("тег «%s»: нижний регистр, буквы, цифры и дефисы", t)
		}
		tags = append(tags, t)
	}
	if nc.Deadline != "" {
		if _, err := time.Parse("2006-01-02", nc.Deadline); err != nil || !reISODate.MatchString(nc.Deadline) {
			return "", errors.New("срок — дата ГГГГ-ММ-ДД")
		}
	}
	folder := path.Clean("/" + strings.TrimSpace(nc.Folder))[1:]
	known := false
	for _, f := range newFolders(v) {
		known = known || f.Path == folder
	}
	if !known {
		return "", errors.New("нет такого раздела")
	}

	// имя файла — латиницей из заголовка, как у остальных карточек; занято — с номером
	base := slug(title)
	if base == "" {
		base = "card"
	}
	rel := ""
	for n := 1; ; n++ {
		name := base
		if n > 1 {
			name = fmt.Sprintf("%s-%d", base, n)
		}
		rel = folder + "/" + name + ".md"
		full, err := cardPath(rel)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(full); errors.Is(err, os.ErrNotExist) && v.Docs[strings.TrimSuffix(rel, ".md")] == nil {
			break
		}
		if n > 50 {
			return "", errors.New("не нашлось свободного имени файла")
		}
	}

	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "type: %s\n", typ)
	fmt.Fprintf(&b, "summary: %s\n", yamlQuote(summary))
	fmt.Fprintf(&b, "tags: [%s]\n", strings.Join(tags, ", "))
	b.WriteString("status: draft\n")
	fmt.Fprintf(&b, "created: %s\n", ymd(now))
	if nc.Deadline != "" {
		fmt.Fprintf(&b, "deadline: %s\n", nc.Deadline)
	}
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# %s\n", title)
	if body := strings.TrimSpace(normalizeEOL(nc.Body)); body != "" {
		b.WriteString("\n" + body + "\n")
	}
	if typ == "goal" && !strings.Contains(nc.Body, goalKRHeading) {
		// заготовка OKR: впиши результаты и цели — виджет «Цели» подхватит
		b.WriteString("\n" + goalKRHeading + "\n\n| Дата | " + goalPlaceholder + " |\n|---|---|\n| Цель | |\n")
	}
	if b.Len() > maxSaveBytes {
		return "", errors.New("слишком большая карточка")
	}

	full, err := cardPath(rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	// O_EXCL: две формы с одним заголовком не затрут друг друга
	f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", errors.New("карточка с таким именем уже есть")
	}
	if _, err := f.WriteString(b.String()); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	id := strings.TrimSuffix(rel, ".md")
	commitFiles("новая карточка: "+id, rel)
	return id, rebuild()
}

// yamlQuote — строка в двойных кавычках YAML. Разбор frontmatter в
// хранилище (indexer.go) экранирование не снимает, поэтому прямые кавычки
// внутри становятся «ёлочками» — по-русски так и пишут, и снимать нечего.
func yamlQuote(s string) string {
	var b strings.Builder
	open := true
	for _, r := range s {
		if r == '"' {
			if open {
				b.WriteRune('«')
			} else {
				b.WriteRune('»')
			}
			open = !open
			continue
		}
		b.WriteRune(r)
	}
	return `"` + b.String() + `"`
}

var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e", 'ж': "zh", 'з': "z", 'и': "i",
	'й': "y", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t",
	'у': "u", 'ф': "f", 'х': "h", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "sch", 'ъ': "", 'ы': "y", 'ь': "",
	'э': "e", 'ю': "yu", 'я': "ya",
}

// slug — имя файла из заголовка: латиница, цифры и дефисы, не длиннее 60.
func slug(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case translit[r] != "" || r == 'ъ' || r == 'ь':
			b.WriteString(translit[r])
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
		if b.Len() >= 60 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}
