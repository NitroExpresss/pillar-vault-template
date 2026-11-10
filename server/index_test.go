// Разбор хранилища: frontmatter, вики-ссылки, обход, линтер. Всё это
// раньше существовало в двух реализациях (Go и Node) и расходилось молча.
package main

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestРазборFrontmatter(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want map[string]any
		body string
	}{
		{
			name: "без frontmatter",
			in:   "# Просто заголовок\n",
			want: nil,
			body: "# Просто заголовок\n",
		},
		{
			name: "скаляры, кавычки и инлайн-массив",
			in:   "---\ntype: fact\nsummary: \"Одна строка\"\ntags: [здоровье/добавки, сердце]\n---\nтело\n",
			want: map[string]any{"type": "fact", "summary": "Одна строка",
				"tags": []string{"здоровье/добавки", "сердце"}},
			body: "тело\n",
		},
		{
			name: "блочный список edges — берём только id",
			in:   "---\nid: card\nedges:\n  - id: other\n    type: derived-from\n  - id: third\n---\n",
			want: map[string]any{"id": "card", "edges": []string{"other", "third"}},
			body: "",
		},
		{
			name: "CRLF из редактора под Windows",
			in:   "---\r\ntype: note\r\ntags: [pkm]\r\n---\r\nтело\r\n",
			want: map[string]any{"type": "note", "tags": []string{"pkm"}},
			body: "тело\r\n",
		},
		{
			name: "пустое значение — это пустой список, а не строка",
			in:   "---\ntags:\nsummary: есть\n---\n",
			want: map[string]any{"tags": []string{}, "summary": "есть"},
			body: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fm, body := parseFrontmatter(c.in)
			if !reflect.DeepEqual(fm, c.want) {
				t.Errorf("frontmatter = %#v, ожидалось %#v", fm, c.want)
			}
			if body != c.body {
				t.Errorf("тело = %q, ожидалось %q", body, c.body)
			}
		})
	}
}

func TestРазборВикиссылок(t *testing.T) {
	body := "Ссылка [[facts/zeolite]], с подписью [[notes/today|сегодня]],\n" +
		"с якорем [[guides/yoga#неделя-1]], встройка ![[design/canvas]],\n" +
		"с расширением [[facts/mumiyo.md]].\n" +
		"В коде не считается: `[[инлайн]]`\n```\n[[в блоке]]\n```\n"
	got := extractWikilinks(body)
	want := []linkRef{
		{ID: "facts/zeolite", Display: "facts/zeolite"},
		{ID: "notes/today", Display: "сегодня"},
		{ID: "guides/yoga", Display: "guides/yoga#неделя-1"},
		{ID: "design/canvas", Display: "design/canvas"},
		{ID: "facts/mumiyo", Display: "facts/mumiyo.md"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ссылки = %#v,\nожидалось %#v", got, want)
	}
}

// lintFixture — хранилище с заранее известными нарушениями устава.
func lintFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files["SOUL.md"] = "# устав\n"
	for rel, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// runLint ловит отчёт линтера: он пишет в stdout, а проверять надо именно текст.
func runLint(t *testing.T, dir string) (bool, string) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	ok := lintVault(dir)
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return ok, string(out)
}

func TestЛинтерЛовитНарушенияУстава(t *testing.T) {
	dir := lintFixture(t, map[string]string{
		"facts/good.md": "---\nid: good\ntype: fact\nsummary: \"годная\"\ntags: [здоровье/добавки]\nstatus: complete\n---\n" +
			"Ссылка на [[notes/нет-такой]] и edge ниже.\n",
		"facts/dup.md": "---\nid: good\ntype: fact\ntags: [бизнес, здоровье, ПлохойТег, ветка/вне-дерева]\n" +
			"edges:\n  - id: несуществующий\n---\n# Дубль\n",
		"notes/nofm.md":  "# Совсем без frontmatter\n",
		"notes/drift.md": "---\ntype: day-card\nsummary: \"дрейф словарей\"\ntags: [дела]\nstatus: pending\n---\n",
		// закрывающий `---` пропущен — парсер берёт разделитель из конца файла
		"notes/open.md": "---\ntype: note\nsummary: \"шапка не закрыта\"\ntags: [pkm]\nstatus: draft\n\n# Тело\n\nтекст\n---\n> цитата\n",
	})
	ok, out := runLint(t, dir)
	if ok {
		t.Fatal("линтер пропустил битое хранилище")
	}
	must := []string{
		"битая ссылка [[notes/нет-такой]]",
		"нет summary",
		`дубль id "good"`,
		`edge на несуществующий id "несуществующий"`,
		"таксономических тегов больше одного",
		"свободный тег не по формату: ПлохойТег",
		"иерархический тег вне дерева: ветка/вне-дерева",
		"нет frontmatter",
		"нет status",
		`type вне устава: "day-card"`,
		`status вне устава: "pending"`,
		`notes/open.md: frontmatter не закрыт`,
	}
	for _, m := range must {
		if !strings.Contains(out, m) {
			t.Errorf("линтер не сказал про «%s».\nОтчёт:\n%s", m, out)
		}
	}
}

func TestЛинтерМолчитНаЧистомХранилище(t *testing.T) {
	dir := lintFixture(t, map[string]string{
		"facts/a.md": "---\nid: a\ntype: fact\nsummary: \"первая\"\ntags: [здоровье, сердце]\nstatus: complete\n---\n[[b]]\n",
		"facts/b.md": "---\nid: b\ntype: fact\nsummary: \"вторая\"\ntags: [pkm]\nstatus: draft\n---\n",
		// шаблоны и точки входа карточками не считаются: у них своих правил нет
		"_sys/templates/Basic.md": "---\ntype: note\n---\nссылка [[a]]\n",
		"README.md":               "# Читай меня\n",
		"AGENTS.md":               "# Точка входа\n",
	})
	ok, out := runLint(t, dir)
	if !ok {
		t.Fatalf("линтер ругается на чистое хранилище:\n%s", out)
	}
	if !strings.Contains(out, "ошибок: 0   предупреждений: 0") {
		t.Errorf("неожиданный итог:\n%s", out)
	}
}

func TestСборкаСвязываетКарточки(t *testing.T) {
	dir := lintFixture(t, map[string]string{
		"facts/a.md":  "---\nid: a\ntype: fact\nsummary: \"первая\"\ntags: [здоровье]\nstatus: complete\n---\n# Первая\nсмотри [[facts/b]]\n",
		"facts/b.md":  "---\nid: b\ntype: fact\nsummary: \"вторая\"\ntags: [здоровье/добавки]\nstatus: draft\n---\n# Вторая\n",
		"viewer/x.md": "не карточка: viewer в обход не входит\n",
	})
	v, err := buildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Index) != 3 { // две карточки + SOUL.md; viewer/ не считается
		t.Fatalf("посчитано %d карточек, ожидалось 3", len(v.Index))
	}
	if v.Generated.IsZero() {
		t.Error("нет метки сборки")
	}
	if _, ok := v.Docs["viewer/x"]; ok {
		t.Error("в модель попал файл из viewer/")
	}
	a, b := v.Docs["facts/a"], v.Docs["facts/b"]
	if a == nil || b == nil {
		t.Fatal("карточки не попали в модель")
	}
	if len(a.Links) != 1 || a.Links[0].ID != "facts/b" {
		t.Errorf("ссылки первой карточки: %#v", a.Links)
	}
	if len(b.Backlinks) != 1 || b.Backlinks[0] != "facts/a" {
		t.Errorf("обратные ссылки второй карточки: %#v", b.Backlinks)
	}
	if v.Tags["здоровье"] != 1 || v.Tags["здоровье/добавки"] != 1 {
		t.Errorf("теги посчитаны неверно: %#v", v.Tags)
	}
}

// ─────────────────── разметка ───────────────────

func TestРендерМаркдауна(t *testing.T) {
	known := map[string]bool{"facts/zeolite": true}
	resolve := func(target string) string {
		if known[target] {
			return target
		}
		if known["facts/"+target] {
			return "facts/" + target
		}
		return ""
	}
	cases := []struct {
		name string
		in   string
		want []string
		bad  []string
	}{
		{
			name: "вики-ссылка становится ссылкой с классом",
			in:   "смотри [[facts/zeolite|Цеолит]]",
			want: []string{`<a class="wikilink" href="/doc/facts/zeolite">Цеолит</a>`},
		},
		{
			name: "битая ссылка остаётся текстом, а не ведёт в никуда",
			in:   "смотри [[несуществующее]]",
			want: []string{"[[несуществующее]]"},
			bad:  []string{"<a class=\"wikilink\""},
		},
		{
			name: "ссылка в коде — не ссылка",
			in:   "```\n[[facts/zeolite]]\n```",
			bad:  []string{"<a class=\"wikilink\""},
		},
		{
			name: "сырой HTML экранируется",
			in:   "<script>alert(1)</script>",
			bad:  []string{"<script>"},
		},
		{
			name: "таблицы и списки задач из GFM",
			in:   "| а | б |\n|---|---|\n| 1 | 2 |\n\n- [x] сделано\n",
			want: []string{"<table>", "<td>1</td>", `type="checkbox"`},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(renderMarkdown(c.in, resolve))
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("нет %q в:\n%s", w, got)
				}
			}
			for _, b := range c.bad {
				if strings.Contains(got, b) {
					t.Errorf("не должно быть %q в:\n%s", b, got)
				}
			}
		})
	}
}

// ─────────────────── дашборд ───────────────────

func TestЖурналСтолпов(t *testing.T) {
	card := "## Столпы\n\n- **Сон** — до полуночи\n\n## Журнал\n\n" +
		"| Дата | Сон | Вода |\n|---|---|---|\n| 2026-01-01 | ✓ | ✗ |\n| 2026-01-02 | ✓ |  |\n"
	log := parseVowLog(card)
	if log == nil {
		t.Fatal("таблица журнала не найдена")
	}
	if len(log.Rows) != 2 || log.Header[1] != "Сон" {
		t.Fatalf("журнал разобран неверно: %#v", log)
	}

	// стрик считаем от «сегодня» тестовых данных, а не от настоящего дня
	day, _ := time.Parse("2006-01-02", "2026-01-02")
	cur, rec, mark := log.streakOf("Сон", day)
	if cur != 2 || rec != 2 || mark != vowHeld {
		t.Errorf("стрик Сна: cur=%d rec=%d mark=%q, ожидалось 2/2/✓", cur, rec, mark)
	}
	// неотмеченный сегодняшний день цепь не рвёт
	day, _ = time.Parse("2006-01-02", "2026-01-03")
	if cur, _, _ := log.streakOf("Сон", day); cur != 2 {
		t.Errorf("неотмеченный сегодняшний день порвал цепь: cur=%d", cur)
	}
	// пропущенный вчерашний — рвёт
	day, _ = time.Parse("2006-01-02", "2026-01-04")
	if cur, _, _ := log.streakOf("Сон", day); cur != 0 {
		t.Errorf("пропуск не порвал цепь: cur=%d", cur)
	}

	// правка строки за дату: существующая правится, новой дописывается
	next, ok := upsertVowRow(card, log.Header, "2026-01-02", map[string]string{"Сон": vowBroke, "Вода": vowHeld})
	if !ok || !strings.Contains(next, "| 2026-01-02 | ✗ | ✓ |") {
		t.Fatalf("строка за дату не переписана:\n%s", next)
	}
	next, ok = upsertVowRow(card, log.Header, "2026-01-05", map[string]string{"Сон": vowHeld})
	if !ok || !strings.Contains(next, "| 2026-01-05 | ✓ |  |") {
		t.Fatalf("новая строка не дописана:\n%s", next)
	}
	if _, ok := upsertVowRow("карточка без таблицы", log.Header, "2026-01-05", nil); ok {
		t.Error("правка удалась там, где таблицы нет")
	}
}

func TestПоследняяЗаписьДневника(t *testing.T) {
	body := "# Дневник\n\n## 2026-01-01 10:00\n\nпервая\n\n## 2026-01-02 22:15\n\nвторая мысль\n\nхвост абзаца\n"
	e := lastDiaryEntry(body)
	if e == nil || e.When != "2026-01-02 22:15" || e.Text != "вторая мысль" {
		t.Fatalf("последняя запись разобрана неверно: %#v", e)
	}
	if lastDiaryEntry("# Дневник\n\nбез записей\n") != nil {
		t.Error("нашлась запись там, где её нет")
	}
}

func TestКолесоБаланса(t *testing.T) {
	body := "## 💪 Здоровье (6/10)\n\n- цель раз\n- [[pillars/inviolable|устав]]\n\n## 💼 Дело (2/10)\n\n- цель два\n"
	sph := parseSpheres(body)
	if len(sph) != 2 {
		t.Fatalf("разобрано сфер: %d", len(sph))
	}
	// от провисающей к сильной: ответ должен стоять первой строкой
	if sph[0].Name != "💼 Дело" || !sph[0].Low {
		t.Errorf("порядок или пометка низкой сферы неверны: %#v", sph[0])
	}
	if sph[1].Pct != 60 {
		t.Errorf("доля здоровья: %d%%, ожидалось 60", sph[1].Pct)
	}
	if len(sph[0].Goals) != 1 || sph[1].Goals[1] != "устав" {
		t.Errorf("цели разобраны неверно: %#v", sph)
	}
}

func TestВставкаДелаВДень(t *testing.T) {
	sec := "# День\n\n## Текущий день\n\n- [ ] первое\n- [x] сделанное\n\nзаметка\n\n## Потом\n"
	if got := insertTodayTask(sec, "второе"); got != "# День\n\n## Текущий день\n\n- [ ] первое\n- [x] сделанное\n- [ ] второе\n\nзаметка\n\n## Потом\n" {
		t.Fatalf("после последнего чекбокса не встало:\n%s", got)
	}
	if got := insertTodayTask("# День\n\n## Текущий день\n", "одно"); got != "# День\n\n## Текущий день\n\n- [ ] одно\n" {
		t.Fatalf("в пустой раздел в конце файла:\n%q", got)
	}
	if got := insertTodayTask("# День\n\nтекст\n", "одно"); got != "# День\n\nтекст\n\n## Текущий день\n\n- [ ] одно\n" {
		t.Fatalf("раздел не заведён:\n%q", got)
	}
}

func TestУбратьИЗакрытьДело(t *testing.T) {
	day := "# День\n\n## Текущий день\n\n- [ ] первое\n- [ ] второе\n\n---\n\n```\n- [ ] второе\n```\n"
	got, ok := removeTodayTask(day, "первое")
	if !ok || got != "# День\n\n## Текущий день\n\n- [ ] второе\n\n---\n\n```\n- [ ] второе\n```\n" {
		t.Fatalf("дело не убрано:\n%s", got)
	}
	got, ok = removeTodayTask(got, "второе")
	if !ok || !strings.Contains(got, "## Текущий день\n\n"+todayPlaceholder+"\n\n---") {
		t.Fatalf("пустой день без заглушки:\n%s", got)
	}
	if !strings.Contains(got, "```\n- [ ] второе\n```") {
		t.Fatal("тронут образец в код-блоке")
	}
	if _, ok := removeTodayTask(day, "нет такого"); ok {
		t.Error("убрано несуществующее")
	}

	// закрытие: все открытые строки вне код-блока, жирный и срок не мешают
	card := "```\n- [ ] позвонить\n```\n- [ ] позвонить\n- [ ] **позвонить** @2026-01-02\n- [ ] другое\n"
	got, ok = markTaskDone(card, "позвонить")
	if !ok || got != "```\n- [ ] позвонить\n```\n- [x] позвонить\n- [x] **позвонить** @2026-01-02\n- [ ] другое\n" {
		t.Fatalf("закрыто не то:\n%s", got)
	}
	if _, ok := markTaskDone("- [x] позвонить\n", "позвонить"); ok {
		t.Error("закрыто уже закрытое")
	}
}

func TestТекстБезРазметки(t *testing.T) {
	body := "# Заголовок\n\nПервая часть протокола **Герсона** и [[facts/x|клевер]], ![[схема]].\n\n" +
		"> цитата _курсивом_\n\n---\n\n- [ ] дело @2026-01-01\n1. шаг [ссылка](https://x.y)\n\n" +
		"| a | b |\n|---|---|\n| 1 | 2 |\n\n```go\nкод()\n```\nsnake_case `x`"
	want := "Заголовок Первая часть протокола Герсона и клевер, схема. цитата курсивом дело @2026-01-01 шаг ссылка a b 1 2 код() snake_case x"
	if got := plainText(body); got != want {
		t.Fatalf("разметка осталась:\n got: %s\nwant: %s", got, want)
	}
}

func TestЗамерВТаблицу(t *testing.T) {
	card := "---\nmetrics: body\ndemo: true\n---\n# Журнал\n\n> ⚠️ **Строки ниже — демонстрационные.** Реален первый.\n> Замени своими.\n\nтекст\n\n" +
		"| Дата | Вес, кг | Талия, см |\n|---|---|---|\n| 2026-05-26 | 110.0 | 118.0 |\n| 2026-06-09 | 104.9 | 114.0 |\n\n## Дальше\n"
	// новая дата — по порядку, между строк
	got, ok := upsertMetricRow(card, "2026-06-02", []string{"106.8", ""})
	if !ok || !strings.Contains(got, "| 2026-05-26 | 110.0 | 118.0 |\n| 2026-06-02 | 106.8 |  |\n| 2026-06-09") {
		t.Fatalf("строка не на месте:\n%s", got)
	}
	// та же дата — замена, пустое берётся из прежней строки
	got, _ = upsertMetricRow(got, "2026-06-09", []string{"", "113.0"})
	if !strings.Contains(got, "| 2026-06-09 | 104.9 | 113.0 |") || strings.Count(got, "2026-06-09") != 1 {
		t.Fatalf("замена:\n%s", got)
	}
	// позже всех — в конец таблицы, до следующего раздела
	got, _ = upsertMetricRow(got, "2026-07-01", []string{"100.0", "110.0"})
	if !strings.Contains(got, "| 2026-07-01 | 100.0 | 110.0 |\n\n## Дальше") {
		t.Fatalf("в конец:\n%s", got)
	}
	// демо: остаются первый замер и всё прочее, уходят флаг и предупреждение
	clean, _ := clearDemo(card)
	if strings.Contains(clean, "demo: true") || strings.Contains(clean, "демонстрацион") || strings.Contains(clean, "Замени своими") {
		t.Fatalf("демо не убрано:\n%s", clean)
	}
	if !strings.Contains(clean, "| 2026-05-26 | 110.0 | 118.0 |\n\n## Дальше") || strings.Contains(clean, "2026-06-09") {
		t.Fatalf("строки таблицы:\n%s", clean)
	}
	if !strings.Contains(clean, "# Журнал\n\nтекст") {
		t.Fatalf("лишние пустые строки на месте предупреждения:\n%q", clean)
	}
}

func TestОкноСниппетаПоГраницамСлов(t *testing.T) {
	body := []rune("хранилище само себе индекс и не может разойтись с реальностью")
	got := wordWindow(body, 4, 30)
	if got != "…само себе индекс и…" {
		t.Errorf("окно не по словам: %q", got)
	}
	if got := wordWindow(body, 0, len(body)); got != string(body) {
		t.Errorf("целый текст изменился: %q", got)
	}
}

func TestПоискСтавитСловоВышеКускаСлова(t *testing.T) {
	if pos, ok := wordHit("таксономия и сон", "сон"); !ok || pos != len("таксономия и ") {
		t.Errorf("начало слова не найдено: %d %v", pos, ok)
	}
	if pos, ok := wordHit("таксономия", "сон"); ok || pos < 0 {
		t.Errorf("кусок слова: %d %v", pos, ok)
	}
	v := &Vault{Index: []*Doc{
		{ID: "a", Title: "Таксономия тегов", Plain: "таксономия тегов"},
		{ID: "b", Title: "Режим", Plain: "сон до полуночи"},
	}}
	hits := search(v, "сон")
	if len(hits) != 2 || hits[0].Doc.ID != "b" {
		t.Fatalf("карточка про сон не первая: %+v", hits)
	}
	if hits[0].Snippet != "сон до полуночи" {
		t.Errorf("выдержка: %q", hits[0].Snippet)
	}
}

func TestЛинтерПринимаетТолькоPinTrue(t *testing.T) {
	dir := lintFixture(t, map[string]string{
		"notes/a.md": "---\ntype: note\nsummary: \"a\"\ntags: [pkm]\nstatus: draft\npin: true\n---\n# A\n",
		"notes/b.md": "---\ntype: note\nsummary: \"b\"\ntags: [pkm]\nstatus: draft\npin: false\n---\n# B\n",
	})
	ok, out := runLint(t, dir)
	if ok || !strings.Contains(out, "notes/b.md: pin только true") || strings.Contains(out, "notes/a.md") {
		t.Fatalf("линтер о pin:\n%s", out)
	}
}

func TestЦелиКакOKR(t *testing.T) {
	if p := pathPct(110, 100, 85); p != 40 {
		t.Errorf("вес вниз: %v", p)
	}
	if p := pathPct(100, 150, 200); p != 50 {
		t.Errorf("доход вверх: %v", p)
	}
	body := "# Цель\n\n## Ключевые результаты\n\n| Дата | Вес, кг | Книги, шт | Заметка |\n|---|---|---|---|\n" +
		"| Цель | 85 | 12 | |\n| 2026-01-15 | | 2 | старт |\n| 2026-03-01 | | 6 | |\n\nтекст\n\n" +
		"## Задачи (повторяющиеся)\n\n- [ ] взвешивание\n\n## Дела (одноразовые)\n\n- [x] купить весы\n- [ ] записаться в зал\n"
	m := &Metrics{Source: "notes/body", Columns: []string{"Дата", "Вес, кг"}, Rows: []metricRow{
		{Date: "2025-12-01", Values: []*float64{parseNum("120")}}, // до старта цели — не в счёт
		{Date: "2026-01-10", Values: []*float64{parseNum("110")}},
		{Date: "2026-03-01", Values: []*float64{parseNum("97.5")}},
	}}
	d := &Doc{ID: "goals/g", Title: "Цель", Type: "goal", Created: "2026-01-01", Body: body,
		FM: map[string]any{"deadline": "2026-07-01"}}
	g := goalOf(&Vault{Metrics: m}, d, "2026-04-01")
	if g == nil || len(g.KRs) != 2 {
		t.Fatalf("результаты: %+v", g)
	}
	w, b := g.KRs[0], g.KRs[1]
	if w.Name != "Вес" || w.Unit != "кг" || w.From != "notes/body" || w.Start != 110 || w.Current != 97.5 || w.Pct != 50 || len(w.Points) != 2 {
		t.Errorf("вес из журнала замеров: %+v", w)
	}
	if b.Name != "Книги" || b.Start != 2 || b.Current != 6 || b.Pct != 40 {
		t.Errorf("книги из таблицы цели: %+v", b)
	}
	// 90 дней из 181 — ~49.7% времени; прогресс (50+40)/2 = 45 — в пределах графика
	if g.Pct != 45 || g.TimePct < 49 || g.TimePct > 50 || g.Pace != "on" || g.DaysLeft != 91 {
		t.Errorf("время и темп: pct=%v time=%v pace=%s left=%d", g.Pct, g.TimePct, g.Pace, g.DaysLeft)
	}
	if g.Tasks != [2]int{1, 2} {
		t.Errorf("дела в карточке: %v", g.Tasks)
	}
	if goalOf(&Vault{}, &Doc{ID: "x", Type: "goal", Body: "# пусто\n"}, "2026-04-01") != nil {
		t.Error("цель без результатов и дел попала в виджет")
	}
}

func TestЦельПравитсяБезMarkdown(t *testing.T) {
	raw := "---\ntype: goal\nsummary: \"s\"\ntags: [цели]\nstatus: draft\n---\n# Подушка\n\n## Ключевые результаты\n\n| Дата | " + goalPlaceholder + " |\n|---|---|\n| Цель | |\n\n## Дела\n\n- [ ] открыть счёт\n"
	// первый результат заменяет заготовку
	got, err := editGoalTable(raw, func(t *krTable) error {
		t.cols[1], t.target = "Подушка, ₽ тыс", []string{"300"}
		return nil
	})
	if err != nil || !strings.Contains(got, "| Дата | Подушка, ₽ тыс |\n|---|---|\n| Цель | 300 |\n\n## Дела") {
		t.Fatalf("таблица:\n%s", got)
	}
	// показания: новая дата — строка, та же дата — дописывается, порядок по дате
	got, _ = editGoalTable(got, func(t *krTable) error {
		t.rows = append(t.rows, []string{"2026-03-01", "120"}, []string{"2026-01-01", "40"})
		return nil
	})
	if !strings.Contains(got, "| Цель | 300 |\n| 2026-01-01 | 40 |\n| 2026-03-01 | 120 |") {
		t.Errorf("порядок показаний:\n%s", got)
	}
	// раздела нет — заводится в конце, остальное не трогается
	got, _ = editGoalTable("# Цель\n\nтекст\n", func(t *krTable) error {
		t.cols = append(t.cols, "Книги, шт")
		t.target = []string{"12"}
		return nil
	})
	if !strings.HasPrefix(got, "# Цель\n\nтекст\n") || !strings.Contains(got, goalKRHeading+"\n\n| Дата | Книги, шт |") {
		t.Errorf("новый раздел:\n%s", got)
	}
	// срок во frontmatter: поставить и снять
	if s, _ := setFM(raw, "deadline", "2026-12-31"); !strings.Contains(s, "status: draft\ndeadline: 2026-12-31\n---") {
		t.Errorf("срок:\n%s", s)
	}
	s, _ := setFM(raw, "deadline", "2026-12-31")
	if s2, _ := setFM(s, "deadline", ""); s2 != raw {
		t.Errorf("срок не снят:\n%s", s2)
	}
}

func TestСтартЦелиСПервогоПоказания(t *testing.T) {
	body := "# Ц\n\n## Ключевые результаты\n\n| Дата | Подушка, ₽ |\n|---|---|\n| Цель | 300 |\n| 2026-09-01 | 40 |\n| 2026-10-15 | 130 |\n"
	d := &Doc{ID: "goals/p", Type: "goal", Created: "2026-10-22", Body: body, FM: map[string]any{"deadline": "2027-03-01"}}
	g := goalOf(&Vault{}, d, "2026-10-22")
	if g.Start != "2026-09-01" || g.TimePct < 28 || g.TimePct > 29 || g.Pace != "on" {
		t.Errorf("старт %s, время %v, темп %s", g.Start, g.TimePct, g.Pace)
	}
	d.FM["start"] = "2026-10-01" // явный старт — уважается
	if g := goalOf(&Vault{}, d, "2026-10-22"); g.Start != "2026-10-01" {
		t.Errorf("явный старт: %s", g.Start)
	}
}

func TestЛинтерСверяетСферы(t *testing.T) {
	dir := lintFixture(t, map[string]string{
		"pillars/life-goals.md": "---\ntype: pillar\nsummary: ц\ntags: [цели]\nstatus: draft\nreviewed: вчера\n---\n## 💪 Здоровье (6/10)\n- чекап\n",
		"goals/ok.md":           "---\ntype: goal\nsummary: в\ntags: [цели]\nstatus: draft\nsphere: здоровье\n---\n",
		"goals/none.md":         "---\ntype: goal\nsummary: без\ntags: [цели]\nstatus: draft\n---\n",
		"plans/bad.md":          "---\ntype: plan\nsummary: мимо\ntags: [цели]\nstatus: draft\nsphere: космос\n---\n",
	})
	_, out := runLint(t, dir)
	for _, m := range []string{
		`plans/bad.md: sphere вне колеса: "космос"`,
		"goals/none.md: нет sphere",
		"reviewed — дата ГГГГ-ММ-ДД",
	} {
		if !strings.Contains(out, m) {
			t.Errorf("линтер не сказал про «%s».\nОтчёт:\n%s", m, out)
		}
	}
	if strings.Contains(out, "goals/ok.md") {
		t.Errorf("линтер ругается на цель со сферой:\n%s", out)
	}
}
