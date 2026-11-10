// Страницы хранилища. Сервер отдаёт готовый HTML: в браузер не уходит ни
// модель хранилища, ни тела карточек — только та страница, которую попросили.
//
// Записи (отметка столпа, запись в дневник, правка карточки) — обычные
// формы: POST, запись в markdown, редирект обратно. Без JSON-API и без
// состояния в браузере.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ─────────────────── данные страницы ───────────────────

type crumb struct {
	Label string
	URL   string
}

type searchHit struct {
	Doc     *Doc
	Snippet string
}

// ─────────────────── обработчики ───────────────────

// docHTML — разметка карточки, одна для прежней страницы и для приложения.
// Первый H1 убираем, если он повторяет заголовок карточки: показывать одно
// и то же дважды незачем. Ведущие пустые строки после frontmatter обычны,
// поэтому ищем не строго с нулевой позиции.
func docHTML(v *Vault, d *Doc) template.HTML {
	body := strings.TrimLeft(d.Body, " \t\r\n")
	if m := reHeading1.FindStringSubmatchIndex(body); m != nil && m[0] == 0 {
		if strings.TrimSpace(body[m[2]:m[3]]) == strings.TrimSpace(d.Title) {
			body = body[m[1]:]
		}
	}
	return renderMarkdown(body, v.resolver())
}

const conflictNotice = "карточка изменилась, пока ты правил: сверь с версией на диске и сохрани ещё раз"

// saveEdit — запись правки из редактора (старого или приложения). Пока
// редактор был открыт, карточку мог поправить агент, git pull или вторая
// вкладка. Молча затереть их правку нельзя: при расхождении версий ничего
// не пишется (conflict), редактор показывает обе версии, повторное
// сохранение со свежей версией пишет осознанно.
func saveEdit(d *Doc, disk, content, ver string) (conflict bool, err error) {
	if ver != "" && ver != cardVersion(disk) {
		return true, nil
	}
	return false, saveCard(d.Path, content, "правка "+d.Path)
}

// cardVersion — отпечаток содержимого карточки для защиты от перезаписи.
// Переводы строк выравниваются: браузер шлёт textarea с CRLF.
func cardVersion(raw string) string {
	sum := sha256.Sum256([]byte(normalizeEOL(raw)))
	return hex.EncodeToString(sum[:8])
}

// Отметка столпа: правим ту же таблицу в pillars/daily-vows.md, которую
// человек правит руками. Источник истины — карточка, не сервер.
func handleVow(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	v := currentVault()
	d := v.Docs[vowsCard]
	if d == nil || v.VowLog == nil {
		http.Redirect(w, r, urlWithMsg("/", "журнал столпов не найден"), http.StatusSeeOther)
		return
	}
	name, mark := r.FormValue("name"), r.FormValue("mark")
	if mark != vowHeld && mark != vowBroke && mark != "" {
		http.Error(w, "неизвестная отметка", http.StatusBadRequest)
		return
	}
	raw, err := readCard(d.Path)
	if err != nil {
		http.Redirect(w, r, urlWithMsg("/", "журнал не прочитан"), http.StatusSeeOther)
		return
	}
	today := ymd(time.Now())
	marks := v.VowLog.marksOn(today)
	if marks[name] == mark { // повторное нажатие снимает отметку
		mark = ""
	}
	marks[name] = mark
	next, ok := upsertVowRow(raw, v.VowLog.Header, today, marks)
	if !ok {
		http.Redirect(w, r, urlWithMsg("/", "в карточке нет таблицы журнала"), http.StatusSeeOther)
		return
	}
	what := "столп «" + name + "» снят"
	if mark != "" {
		what = "столп «" + name + "» " + mark
	}
	if err := saveCard(d.Path, next, what); err != nil {
		http.Redirect(w, r, urlWithMsg("/", "отметка не сохранена"), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleDiary(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	v := currentVault()
	d := v.Docs[diaryCard]
	text := strings.TrimSpace(normalizeEOL(r.FormValue("text")))
	if d == nil || text == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	raw, err := readCard(d.Path)
	if err != nil {
		http.Redirect(w, r, urlWithMsg("/", "дневник не прочитан"), http.StatusSeeOther)
		return
	}
	now := time.Now()
	entry := fmt.Sprintf("\n\n## %s %s\n\n%s\n", ymd(now), now.Format("15:04"), text)
	if err := saveCard(d.Path, strings.TrimRight(raw, " \t\r\n")+entry, "запись в дневник"); err != nil {
		http.Redirect(w, r, urlWithMsg("/", "запись не сохранена"), http.StatusSeeOther)
		return
	}
	markOK(w)
	http.Redirect(w, r, urlWithMsg("/", "записано"), http.StatusSeeOther)
}

// ─────────────────── общее ───────────────────

func requirePage(w http.ResponseWriter, r *http.Request) bool {
	if isAdmin(r) {
		return true
	}
	// Виджеты Android шлют формы мимо WebView. Им редирект на «/» не годится:
	// успех отвечает тем же редиректом, и протухшая сессия выглядела бы как
	// «записано», хотя не записалось ничего.
	if r.Header.Get(appHeader) != "" || wantsAPI(r) {
		http.Error(w, "нужен вход", http.StatusUnauthorized)
		return false
	}
	// гостю показывать нечего: за формой входа пусто
	http.Redirect(w, r, "/", http.StatusSeeOther)
	return false
}

func requirePost(w http.ResponseWriter, r *http.Request) bool {
	if !requirePage(w, r) {
		return false
	}
	if r.Method != http.MethodPost {
		http.Error(w, "только POST", http.StatusMethodNotAllowed)
		return false
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "битая форма", http.StatusBadRequest)
		return false
	}
	return true
}

func docCrumbs(v *Vault, d *Doc) []crumb {
	out := []crumb{{Label: "Vault", URL: "/"}}
	if d.Folder == "." {
		return out
	}
	acc := []string{}
	for _, p := range strings.Split(d.Folder, "/") {
		acc = append(acc, p)
		path := strings.Join(acc, "/")
		out = append(out, crumb{Label: folderLabel(v, path), URL: folderURL(path)})
	}
	return out
}

func sortByTitle(list []*Doc) []*Doc {
	sort.SliceStable(list, func(i, j int) bool {
		return strings.ToLower(list[i].Title) < strings.ToLower(list[j].Title)
	})
	return list
}

// resolver — как индексатор: точное совпадение id, иначе по имени файла.
func (v *Vault) resolver() func(string) string {
	byBasename := map[string][]string{}
	for id := range v.Docs {
		b := strings.ToLower(id[strings.LastIndex(id, "/")+1:])
		byBasename[b] = append(byBasename[b], id)
	}
	return func(target string) string { return resolveTarget(target, v.Docs, byBasename) }
}

func folderLabel(v *Vault, path string) string {
	if path == "." {
		return "Корень"
	}
	top := strings.Split(path, "/")[0]
	label := top
	if m, ok := v.FolderMeta[top]; ok && m.Label != "" {
		label = m.Label
	}
	if strings.Contains(path, "/") {
		label += " / " + strings.Join(strings.Split(path, "/")[1:], " / ")
	}
	return label
}

// search — тот же счёт, что был в браузере: заголовок весит больше summary,
// summary больше тегов, полный текст — меньше всего.
func search(v *Vault, q string) []searchHit {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	terms := strings.Fields(q)
	type scored struct {
		d          *Doc
		word, part int // попадания с начала слова и куском внутри слова
		snip       string
	}
	out := []scored{}
	for _, d := range v.Index {
		if d.Status == "archived" { // архив — убрано с глаз; найти можно в разделе
			continue
		}
		hay := strings.ToLower(d.Title + " " + strings.Join(d.Tags, " ") + " " + d.Type + " " + d.Folder)
		full := strings.ToLower(d.Plain) // без разметки: «**» и «##» не находятся и не лезут в выдержку
		summ := strings.ToLower(d.Summary)
		sc := scored{d: d}
		add := func(s, t string, w int) {
			if pos, ok := wordHit(s, t); ok {
				sc.word += w
			} else if pos >= 0 {
				sc.part += w
			}
		}
		for _, t := range terms {
			add(strings.ToLower(d.Title), t, 10)
			add(summ, t, 6)
			add(strings.ToLower(strings.Join(d.Tags, " ")), t, 5)
			add(hay, t, 2)
			add(full, t, 1)
		}
		if sc.word+sc.part == 0 {
			continue
		}
		sc.snip = searchSnippet(d, full, summ, terms[0])
		out = append(out, sc)
	}
	// слово, начатое запросом, важнее любого числа кусков внутри слов:
	// иначе «сон» поднимал «Герсона» и «таксономию» выше карточек про сон
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].word != out[j].word {
			return out[i].word > out[j].word
		}
		return out[i].part > out[j].part
	})
	hits := []searchHit{}
	for i, o := range out {
		if i >= 40 {
			break
		}
		hits = append(hits, searchHit{Doc: o.d, Snippet: o.snip})
	}
	return hits
}

// wordHit — где в s встречается t: первое вхождение с начала слова, а если
// такого нет — первое любое (ok=false); -1 — не встречается вовсе.
func wordHit(s, t string) (pos int, ok bool) {
	first := -1
	for from := 0; ; {
		i := strings.Index(s[from:], t)
		if i < 0 {
			return first, false
		}
		i += from
		if first < 0 {
			first = i
		}
		r, _ := utf8.DecodeLastRuneInString(s[:i])
		if i == 0 || !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return i, true
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		from = i + size
	}
}

// searchSnippet — summary при попадании в него, иначе окно вокруг совпадения.
// Окно считаем в символах: байтовые границы разрубили бы кириллицу пополам,
// и страница уехала бы к браузеру с битой кодировкой.
func searchSnippet(d *Doc, full, summ, term string) string {
	if _, ok := wordHit(summ, term); ok {
		return d.Summary
	}
	pos, ok := wordHit(full, term)
	if pos < 0 {
		if strings.Contains(summ, term) {
			return d.Summary
		}
		return ""
	}
	if !ok && strings.Contains(summ, term) {
		return d.Summary
	}
	body := []rune(d.Plain)
	start := utf8.RuneCountInString(full[:pos]) - 40
	if start < 0 {
		start = 0
	}
	end := start + 120
	if end > len(body) {
		end = len(body)
	}
	return wordWindow(body, start, end)
}

// wordWindow — окно текста по границам слов: обрубок «илище само себе»
// читается как ошибка. Обрезанный край помечается многоточием.
func wordWindow(body []rune, start, end int) string {
	if start > 0 {
		for i := start; i < end && i-start < 20; i++ {
			if unicode.IsSpace(body[i-1]) {
				start = i
				break
			}
		}
	}
	if end < len(body) {
		for i := end; i > start && end-i < 20; i-- {
			if unicode.IsSpace(body[i]) {
				end = i
				break
			}
		}
	}
	out := strings.TrimSpace(string(body[start:end]))
	if start > 0 {
		out = "…" + out
	}
	if end < len(body) {
		out += "…"
	}
	return out
}

func normalizeEOL(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

func urlWithMsg(base, msg string) string { return base + "?m=" + url.QueryEscape(msg) }

// ─────────────────── дело на сегодня ───────────────────

// handleToday правит день с дашборда. do=add — дело из общего списка встаёт
// в «Текущий день» tasks/day; do=new — своё дело из поля пишется туда же;
// do=done — дело закрывается там, где оно живёт
// (в исходной карточке, а копия уходит из дня); do=remove — дело уходит из
// дня, но остаётся открытым в своей карточке. Принимается только текст
// живого дела: форма не может дописать в карточки что угодно.
func handleToday(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	v := currentVault()
	day := v.Docs[dayCard]
	if day == nil {
		http.Redirect(w, r, urlWithMsg("/", "нет карточки "+dayCard), http.StatusSeeOther)
		return
	}
	text := strings.TrimSpace(r.FormValue("text"))
	fail := func(msg string) { http.Redirect(w, r, urlWithMsg("/", msg), http.StatusSeeOther) }

	switch r.FormValue("do") {
	case "", "add":
		if !hasTask(flatten(pickableTasks(v)), text) {
			fail("такого дела нет в списке")
			return
		}
		if err := editCard(day.Path, "на сегодня: "+text, func(raw string) (string, bool) { return insertTodayTask(raw, text), true }); err != nil {
			fail("дело не добавлено")
			return
		}
	case "new":
		// своё дело, набранное в поле: одна строка, без переносов и с потолком
		// длины — иначе в карточку можно было бы вписать заголовок или таблицу
		text = strings.Join(strings.Fields(text), " ")
		if text == "" {
			fail("пустое дело")
			return
		}
		if utf8.RuneCountInString(text) > maxTaskRunes {
			fail("слишком длинное дело")
			return
		}
		if hasTask(todayList(v), text) {
			fail("это дело уже на сегодня")
			return
		}
		if err := editCard(day.Path, "новое на сегодня: "+text, func(raw string) (string, bool) { return insertTodayTask(raw, text), true }); err != nil {
			fail("дело не добавлено")
			return
		}
	case "remove":
		if !hasTask(todayList(v), text) {
			fail("этого дела нет на сегодня")
			return
		}
		if err := editCard(day.Path, "убрано из дня: "+text, func(raw string) (string, bool) { return removeTodayTask(raw, text) }); err != nil {
			fail("дело не убрано")
			return
		}
	case "later":
		if !hasTask(todayList(v), text) {
			fail("этого дела нет на сегодня")
			return
		}
		if msg := laterFromDay(v, text); msg != "" {
			fail(msg)
			return
		}
	case "done":
		if !hasTask(todayList(v), text) {
			fail("этого дела нет на сегодня")
			return
		}
		// закрываем в исходной карточке, если дело пришло оттуда; иначе — в самом дне
		if home := taskHome(v, taskItem{Text: text, Src: dayCard}); home != dayCard {
			if err := editCard(v.Docs[home].Path, "сделано: "+text, func(raw string) (string, bool) { return markTaskDone(raw, text) }); err != nil {
				fail("дело не закрыто")
				return
			}
			if err := editCard(day.Path, "сделано, из дня: "+text, func(raw string) (string, bool) { return removeTodayTask(raw, text) }); err != nil {
				fail("дело закрыто, но осталось в дне")
				return
			}
		} else if err := editCard(day.Path, "сделано: "+text, func(raw string) (string, bool) { return markTaskDone(raw, text) }); err != nil {
			fail("дело не закрыто")
			return
		}
	default:
		http.Error(w, "неизвестное действие", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// maxTaskRunes — потолок своего дела из поля «Сегодня»: дело, а не заметка.
const maxTaskRunes = 300

// editCard — прочитать, поправить, сохранить. Правка, не нашедшая строки,
// — ошибка, а не молчаливое «сохранено».
func editCard(rel, what string, edit func(string) (string, bool)) error {
	raw, err := readCard(rel)
	if err != nil {
		return err
	}
	next, ok := edit(raw)
	if !ok {
		return errors.New("строка не найдена")
	}
	return saveCard(rel, next, what)
}

func hasTask(items []taskItem, text string) bool {
	for _, i := range items {
		if i.Text == text {
			return true
		}
	}
	return false
}

func flatten(groups []taskGroup) []taskItem {
	out := []taskItem{}
	for _, g := range groups {
		out = append(out, g.Items...)
	}
	return out
}

// taskHome — карточка, где дело живёт на самом деле. Дело дня — копия строки
// из бэклога или другой карточки tasks/; клик по нему ведёт туда, и закрывать
// его надо там же. Открытая копия важнее закрытой: дело могли повторить.
func taskHome(v *Vault, it taskItem) string {
	if v.Tasks == nil || it.Src != dayCard {
		return it.Src
	}
	home := ""
	for _, i := range v.Tasks.Items {
		if i.Src == dayCard || i.Text != it.Text {
			continue
		}
		if !i.Done {
			return i.Src
		}
		if home == "" {
			home = i.Src
		}
	}
	if home == "" {
		return it.Src
	}
	return home
}

func todaySpan(lines []string) (head, end int) { return sectionSpan(lines, todaySection) }

// sectionSpan — границы раздела с заголовком heading: строка заголовка и
// первая строка после раздела (следующий заголовок, черта или конец файла).
func sectionSpan(lines []string, heading string) (head, end int) {
	head = -1
	for i, l := range lines {
		if strings.TrimSpace(l) == heading {
			head = i
			break
		}
	}
	if head == -1 {
		return -1, -1
	}
	end = head + 1
	for ; end < len(lines); end++ {
		t := strings.TrimSpace(lines[end])
		if strings.HasPrefix(t, "#") || t == "---" {
			break
		}
	}
	return head, end
}

// insertTodayTask дописывает «- [ ] дело» в конец раздела «Текущий день»:
// после последнего чекбокса раздела, заглушку «_Пусто…_» убирает. Нет
// раздела — заводит его в конце карточки.
func insertTodayTask(raw, text string) string {
	return insertInSection(raw, todaySection, "- [ ] "+text)
}

// insertInSection дописывает строку-чекбокс item в конец раздела heading —
// после последнего чекбокса, убрав заглушку «_Пусто…_». Раздела нет — он
// заводится перед служебным хвостом карточки (первой «---» после последнего
// раздела «##»), а если хвоста нет — в конце.
func insertInSection(raw, heading, item string) string {
	lines := strings.Split(normalizeEOL(raw), "\n")
	head, end := sectionSpan(lines, heading)
	if head == -1 {
		last := -1
		for i, l := range lines {
			if strings.HasPrefix(l, "## ") {
				last = i
			}
		}
		if last >= 0 {
			for i := last + 1; i < len(lines); i++ {
				if strings.TrimSpace(lines[i]) == "---" {
					block := []string{heading, "", item, ""}
					return strings.Join(append(append(append([]string{}, lines[:i]...), block...), lines[i:]...), "\n")
				}
			}
		}
		return strings.TrimRight(strings.Join(lines, "\n"), " \t\n") + "\n\n" + heading + "\n\n" + item + "\n"
	}
	sec := []string{}
	for _, l := range lines[head+1 : end] {
		if !strings.HasPrefix(strings.TrimSpace(l), "_Пусто") {
			sec = append(sec, l)
		}
	}
	at := 0 // после последнего чекбокса; чекбоксов нет — первой строкой
	for j, l := range sec {
		if reTaskLine.MatchString(l) {
			at = j + 1
		}
	}
	sec = append(sec[:at], append([]string{item}, sec[at:]...)...)
	return joinSection(lines, head, end, sec)
}

// removeTodayTask убирает открытое дело из «Текущего дня». Раздел опустел —
// возвращается заглушка, как в свежей карточке.
func removeTodayTask(raw, text string) (string, bool) {
	lines := strings.Split(normalizeEOL(raw), "\n")
	head, end := todaySpan(lines)
	if head == -1 {
		return raw, false
	}
	sec, found := []string{}, false
	for _, l := range lines[head+1 : end] {
		if t, _, done, ok := parseTaskLine(l); ok && !done && t == text && !found {
			found = true
			continue
		}
		sec = append(sec, l)
	}
	if !found {
		return raw, false
	}
	empty := true
	for _, l := range sec {
		if strings.TrimSpace(l) != "" {
			empty = false
		}
	}
	if empty {
		sec = []string{todayPlaceholder}
	}
	return joinSection(lines, head, end, sec), true
}

// joinSection собирает карточку обратно: заголовок, пустая строка, тело
// раздела без пустых краёв, пустая строка, остаток файла.
func joinSection(lines []string, head, end int, sec []string) string {
	for len(sec) > 0 && strings.TrimSpace(sec[0]) == "" {
		sec = sec[1:]
	}
	for len(sec) > 0 && strings.TrimSpace(sec[len(sec)-1]) == "" {
		sec = sec[:len(sec)-1]
	}
	out := append(append([]string{}, lines[:head+1]...), "")
	out = append(out, sec...)
	out = append(out, "") // пустая строка перед следующим разделом или перевод строки в конце файла
	return strings.Join(append(out, lines[end:]...), "\n")
}

// markTaskDone ставит [x] каждому открытому делу с этим текстом: одно дело,
// записанное в карточке дважды (со сроком и без), закрывается целиком, иначе
// копия тут же вернулась бы в список выбора. Код-блоки пропускаются: там
// образцы формата, а не дела.
func markTaskDone(raw, text string) (string, bool) {
	lines := strings.Split(normalizeEOL(raw), "\n")
	fence, found := false, false
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
			continue
		}
		if fence {
			continue
		}
		if t, _, done, ok := parseTaskLine(l); ok && !done && t == text {
			lines[i] = strings.Replace(l, "[ ]", "[x]", 1)
			found = true
		}
	}
	if !found {
		return raw, false
	}
	return strings.Join(lines, "\n"), true
}

// ─────────────────── учёт дел ───────────────────

// handleTask отмечает дело в его карточке прямо из «Учёта дел»: do=done —
// закрыть (копия в «Сегодня», если есть, уходит из дня), do=open — открыть
// снова. Дело ищется по карточке и тексту в модели: чужого не закрыть.
func handleTask(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	v := currentVault()
	src, text, do := r.FormValue("src"), strings.TrimSpace(r.FormValue("text")), r.FormValue("do")
	fail := func(msg string) { http.Redirect(w, r, urlWithMsg("/", msg), http.StatusSeeOther) }
	if do != "done" && do != "open" && do != "move" {
		http.Error(w, "неизвестное действие", http.StatusBadRequest)
		return
	}
	d := v.Docs[src]
	known := false
	if v.Tasks != nil && d != nil && src != dayCard {
		for _, i := range v.Tasks.Items {
			if i.Src == src && i.Text == text && i.Done == (do == "open") { // move — только открытое
				known = true
				break
			}
		}
	}
	if !known {
		fail("такого дела нет")
		return
	}
	if do == "move" {
		date := r.FormValue("date")
		if date == "" {
			date = tomorrow()
		}
		if _, err := time.Parse("2006-01-02", date); err != nil || !reISODate.MatchString(date) {
			fail("нет такой даты")
			return
		}
		if msg := moveTask(v, src, text, date); msg != "" {
			fail(msg)
			return
		}
		http.Redirect(w, r, localBack(r.FormValue("back")), http.StatusSeeOther)
		return
	}
	if do == "open" {
		if err := editCard(d.Path, "снова открыто: "+text, func(raw string) (string, bool) { return markTaskOpen(raw, text) }); err != nil {
			fail("дело не открыто")
		} else {
			http.Redirect(w, r, localBack(r.FormValue("back")), http.StatusSeeOther)
		}
		return
	}
	if err := editCard(d.Path, "сделано: "+text, func(raw string) (string, bool) { return markTaskDone(raw, text) }); err != nil {
		fail("дело не закрыто")
		return
	}
	if day := v.Docs[dayCard]; day != nil && hasTask(todayList(v), text) {
		if err := editCard(day.Path, "сделано, из дня: "+text, func(raw string) (string, bool) { return removeTodayTask(raw, text) }); err != nil {
			fail("дело закрыто, но осталось в дне")
			return
		}
	}
	http.Redirect(w, r, localBack(r.FormValue("back")), http.StatusSeeOther)
}

// markTaskOpen снимает [x] с первого закрытого дела с этим текстом —
// отмена случайного нажатия. Код-блоки пропускаются.
func markTaskOpen(raw, text string) (string, bool) {
	lines := strings.Split(normalizeEOL(raw), "\n")
	fence := false
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
			continue
		}
		if fence {
			continue
		}
		if t, _, done, ok := parseTaskLine(l); ok && done && t == text {
			if j := strings.Index(l, "[x]"); j >= 0 {
				lines[i] = l[:j] + "[ ]" + l[j+3:]
			} else {
				lines[i] = strings.Replace(l, "[X]", "[ ]", 1)
			}
			return strings.Join(lines, "\n"), true
		}
	}
	return raw, false
}

// localBack — куда вернуться после формы: только свой адрес от корня.
// «//чужой.сайт» и «https://…» отбрасываются — иначе форма стала бы
// открытым редиректом.
func localBack(back string) string {
	if !strings.HasPrefix(back, "/") || strings.HasPrefix(back, "//") || strings.HasPrefix(back, "/\\") {
		return "/"
	}
	return back
}
