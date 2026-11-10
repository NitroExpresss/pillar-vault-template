// Pillar Vault — линтер хранилища (операция Lint из concepts/ai-pkm.md).
//
// Ошибки (код выхода 1): битые [[вики-ссылки]], edges на несуществующие id,
// дубли id, карточка без summary, нарушение таксономии тегов.
// Предупреждения (код 0): нет tags / type / status.
//
// Запуск:  go run ./server -root . -lint   (на сервере: pillar-vault -lint)
//
// Живёт рядом с индексатором намеренно: обход хранилища, разбор frontmatter
// и вики-ссылок у них общие. Пока линтер был на Node, эти правила
// существовали в двух реализациях и успели разойтись.
package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// точки входа агента и журнал — это не карточки, устав к ним не применяется
var notCards = map[string]bool{
	"README.md":   true,
	"SOUL.md":     true,
	"AGENTS.md":   true,
	"CLAUDE.md":   true,
	"_sys/log.md": true,
}

// шаблоны: у них проверяются только ссылки — своих summary и тегов у них нет
const templateDir = "_sys/templates"

// taxonomy — фиксированное дерево первого тега. Источник истины — SOUL.md,
// раздел «Теги»; новая ветка заводится приказом хозяина и правкой здесь.
var taxonomy = map[string]bool{
	"здоровье": true, "здоровье/добавки": true, "здоровье/практики": true,
	"бизнес": true, "бизнес/it": true, "бизнес/project-mapping": true,
	"pkm": true, "дела": true, "цели": true,
}

// свободный тег: нижний регистр, дефисы вместо пробелов
var reFreeTag = regexp.MustCompile(`^[a-zа-яё0-9][a-zа-яё0-9-]*$`)

var reHeading = regexp.MustCompile(`^#{1,6} `)

// словари type и status закрыты так же, как дерево тегов (SOUL.md,
// «Хранилище»). Пока линтер сверял только наличие, они разошлись:
// day-card, todo, pending, forming накопились незаметно.
var cardTypeSet = map[string]bool{
	"fact": true, "supplement": true, "guide": true, "hypothesis": true, "concept": true,
	"pillar": true, "source": true, "note": true, "question": true, "area": true,
	"plan": true, "goal": true, "task": true, "archive": true,
}
var cardStatusSet = map[string]bool{
	"draft": true, "researching": true, "active": true, "complete": true, "archived": true,
}

type lintDoc struct {
	rel  string
	fm   map[string]any
	head string // сырой текст frontmatter — по нему ловится незакрытая шапка
	body string
}

// unclosedHead — строка шапки, которая на YAML не похожа: заголовок или текст
// без двоеточия. Так выглядит frontmatter без закрывающего `---`: парсер берёт
// разделитель из конца файла и съедает тело целиком.
func unclosedHead(head string) string {
	for _, raw := range strings.Split(head, "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if strings.TrimSpace(line) == "" || line[0] == ' ' || line[0] == '\t' {
			continue
		}
		if reHeading.MatchString(line) || !strings.Contains(line, ":") {
			return line
		}
	}
	return ""
}

// lintVault печатает отчёт и возвращает false, если найдены ошибки.
func lintVault(vaultRoot string) bool {
	cards, errs, warns, err := lintCheck(vaultRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return false
	}
	for _, e := range errs {
		fmt.Printf("  ✖ %s\n", e)
	}
	for _, w := range warns {
		fmt.Printf("  ⚠ %s\n", w)
	}
	fmt.Printf("\nкарточек: %d   ошибок: %d   предупреждений: %d\n", cards, len(errs), len(warns))
	return len(errs) == 0
}

// lintCheck — проверка хранилища без печати: число карточек, ошибки и
// предупреждения строками «файл: что не так». Её же показывает приложение
// («Здоровье хранилища»), чтобы без агента чинить самому.
func lintCheck(vaultRoot string) (cards int, errs, warns []string, err error) {
	files, err := walkMarkdown(vaultRoot)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("обход хранилища: %v", err)
	}

	docs := make([]lintDoc, 0, len(files))
	knownIDs := map[string]bool{}   // id из frontmatter, пути без .md и имена файлов
	byBasename := map[string]bool{} // по имени файла ссылку тоже разрешаем
	for _, full := range files {
		raw, err := os.ReadFile(full)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("чтение %s: %v", full, err)
		}
		rel, err := filepath.Rel(vaultRoot, full)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("путь %s: %v", full, err)
		}
		relSlash := filepath.ToSlash(rel)
		pathID := strings.TrimSuffix(relSlash, path.Ext(relSlash))
		fm, body := parseFrontmatter(string(raw))
		head := ""
		if m := reFrontmatter.FindStringSubmatch(string(raw)); m != nil {
			head = m[1]
		}
		docs = append(docs, lintDoc{rel: relSlash, fm: fm, head: head, body: body})

		knownIDs[pathID] = true
		base := strings.ToLower(path.Base(pathID))
		knownIDs[base] = true
		byBasename[base] = true
		if id := fmString(fm, "id"); id != "" {
			knownIDs[id] = true
		}
	}

	seenIDs := map[string]string{} // id → в какой карточке он уже встречался

	// сферы — заголовки колеса в life-goals; нет карточки — нечем и сверять
	spheres := map[string]bool{}
	for _, d := range docs {
		if d.rel == goalsCard+".md" {
			for _, s := range parseSpheres(d.body) {
				spheres[s.Key] = true
			}
		}
	}

	for _, d := range docs {
		// битые вики-ссылки проверяем везде, включая шаблоны и README
		for _, l := range extractWikilinks(d.body) {
			if knownIDs[l.ID] || byBasename[strings.ToLower(path.Base(l.ID))] {
				continue
			}
			errs = append(errs, fmt.Sprintf("%s: битая ссылка [[%s]]", d.rel, l.ID))
		}

		if notCards[d.rel] || strings.HasPrefix(d.rel, templateDir) {
			continue
		}
		if d.fm == nil {
			errs = append(errs, d.rel+": нет frontmatter")
			continue
		}
		if fmString(d.fm, "summary") == "" {
			errs = append(errs, d.rel+": нет summary")
		}
		if bad := unclosedHead(d.head); bad != "" {
			errs = append(errs, fmt.Sprintf("%s: frontmatter не закрыт — нет второго \"---\" (тело ушло в шапку: %q)", d.rel, clip(bad, 40)))
		}
		if t := fmString(d.fm, "type"); t == "" {
			warns = append(warns, d.rel+": нет type")
		} else if !cardTypeSet[t] {
			errs = append(errs, fmt.Sprintf("%s: type вне устава: %q", d.rel, t))
		}
		if s := fmString(d.fm, "status"); s == "" {
			warns = append(warns, d.rel+": нет status")
		} else if !cardStatusSet[s] {
			errs = append(errs, fmt.Sprintf("%s: status вне устава: %q", d.rel, s))
		}
		// каждая цель и каждый план отвечают сфере колеса (этап 4.1)
		if sp := strings.ToLower(fmString(d.fm, "sphere")); len(spheres) > 0 {
			t := fmString(d.fm, "type")
			switch {
			case sp != "" && !spheres[sp]:
				errs = append(errs, fmt.Sprintf("%s: sphere вне колеса: %q — сферы в [[%s]]", d.rel, sp, goalsCard))
			case sp == "" && (t == "goal" || t == "plan") && fmString(d.fm, "status") != "archived":
				warns = append(warns, d.rel+": нет sphere — какой сфере колеса отвечает")
			}
		}
		if rv, ok := d.fm["reviewed"]; ok && !reISODate.MatchString(fmt.Sprint(rv)) {
			errs = append(errs, fmt.Sprintf("%s: reviewed — дата ГГГГ-ММ-ДД (сейчас %v)", d.rel, rv))
		}
		if dl, ok := d.fm["deadline"]; ok && !reISODate.MatchString(fmt.Sprint(dl)) {
			errs = append(errs, fmt.Sprintf("%s: deadline — дата ГГГГ-ММ-ДД (сейчас %v)", d.rel, dl))
		}
		if p, ok := d.fm["pin"]; ok && p != "true" {
			errs = append(errs, fmt.Sprintf("%s: pin только true — открепить значит убрать строку (сейчас %v)", d.rel, p))
		}

		// дубли id: два хозяина у одного имени — и ссылки ведут наугад
		if id := fmString(d.fm, "id"); id != "" {
			if prev, ok := seenIDs[id]; ok {
				errs = append(errs, fmt.Sprintf("%s: дубль id %q (уже в %s)", d.rel, id, prev))
			} else {
				seenIDs[id] = d.rel
			}
		}

		// edges ведут на существующие карточки
		if edges, ok := d.fm["edges"].([]string); ok {
			for _, e := range edges {
				if e != "" && !knownIDs[e] {
					errs = append(errs, fmt.Sprintf("%s: edge на несуществующий id %q", d.rel, e))
				}
			}
		}

		// таксономия: ровно один тег из дерева, и он первый
		tags, _ := d.fm["tags"].([]string)
		if len(tags) == 0 {
			warns = append(warns, d.rel+": нет tags")
			continue
		}
		var tax []string
		for _, t := range tags {
			if taxonomy[t] {
				tax = append(tax, t)
			}
		}
		switch {
		case len(tax) == 0:
			errs = append(errs, fmt.Sprintf("%s: нет таксономического тега (%s)", d.rel, strings.Join(tags, ", ")))
		case len(tax) > 1:
			errs = append(errs, fmt.Sprintf("%s: таксономических тегов больше одного: %s", d.rel, strings.Join(tax, ", ")))
		case tags[0] != tax[0]:
			errs = append(errs, fmt.Sprintf("%s: таксономический тег не первый: %s", d.rel, tax[0]))
		}
		for _, t := range tags {
			switch {
			case taxonomy[t]:
			case strings.Contains(t, "/"):
				errs = append(errs, fmt.Sprintf("%s: иерархический тег вне дерева: %s", d.rel, t))
			case !reFreeTag.MatchString(t):
				errs = append(errs, fmt.Sprintf("%s: свободный тег не по формату: %s", d.rel, t))
			}
		}
	}

	return len(docs), errs, warns, nil
}
