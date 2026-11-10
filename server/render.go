// Markdown → HTML на сервере.
//
// Раньше это делал marked.js в браузере. Перенос сюда убрал из стека целый
// слой: сборку хранилища больше незачем возить в браузер, а разбор карточки
// живёт там же, где сама карточка.
//
// Сырой HTML в разметке намеренно экранируется (goldmark без WithUnsafe):
// в карточках его нет, а страница с чужим тегом внутри — ровно тот случай,
// ради которого стоит терпеть строгость.
package main

import (
	"bytes"
	"html/template"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM), // таблицы, списки задач, зачёркивание
	goldmark.WithRendererOptions(
		html.WithHardWraps(), // перевод строки в карточке — это перевод строки
		html.WithXHTML(),
	),
)

// docURL — адрес карточки. Экранируем посегментно: в именах карточек есть
// и пробелы («Wim Hof Breathing»), и кириллица.
func docURL(id string) string {
	parts := strings.Split(id, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return "/doc/" + strings.Join(parts, "/")
}

func folderURL(path string) string { return "/folder/" + url.PathEscape(path) }
func tagURL(tag string) string     { return "/tag/" + url.PathEscape(tag) }

var (
	reMdImage    = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	reMdLink     = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	reMdLineHead = regexp.MustCompile(`(?m)^[ \t]*(?:#{1,6}[ \t]+|>[ \t]?|(?:[-*+]|\d+[.)])[ \t]+(?:\[[ xX]\][ \t]+)?)`)
	reMdTableRul = regexp.MustCompile(`(?m)^[ \t]*\|?[ \t]*:?-{2,}:?[ \t]*(?:\|[ \t]*:?-{2,}:?[ \t]*)*\|?[ \t]*$`)
	reMdFence    = regexp.MustCompile("(?m)^[ \t]*```.*$")
	reMdRule     = regexp.MustCompile(`(?m)^[ \t]*(?:[-*_][ \t]*){3,}$`)
	reMdEmphUnd  = regexp.MustCompile(`(^|[\s(])_+|_+([\s).,!?:;]|$)`)
	reHTMLTag    = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)
)

// plainText — карточка без разметки, одной строкой: для выдержек в поиске
// и списках. Раньше выдержка резалась из сырого markdown, и в выдачу
// приезжали «**», «##» и «[[…]]».
func plainText(body string) string {
	s := reMdFence.ReplaceAllString(body, "")
	s = reMdTableRul.ReplaceAllString(s, "")
	s = reMdRule.ReplaceAllString(s, "")
	s = unlinkWiki(strings.ReplaceAll(s, "![[", "[["))
	s = reMdImage.ReplaceAllString(s, "$1")
	s = reMdLink.ReplaceAllString(s, "$1")
	s = reMdLineHead.ReplaceAllString(s, "")
	s = reHTMLTag.ReplaceAllString(s, "")
	s = strings.NewReplacer("**", "", "~~", "", "`", "", "*", "", "|", " ").Replace(s)
	s = reMdEmphUnd.ReplaceAllString(s, "$1$2")
	return strings.TrimSpace(normalizeSpaces(s))
}

// renderMarkdown разбирает карточку в HTML. resolve превращает цель
// [[вики-ссылки]] в id существующей карточки; пустая строка — ссылка битая.
func renderMarkdown(body string, resolve func(string) string) template.HTML {
	var buf bytes.Buffer
	if err := md.Convert([]byte(linkWikilinks(body, resolve)), &buf); err != nil {
		// Ошибка разбора не должна ронять страницу: показываем текст как есть.
		return template.HTML("<pre>" + template.HTMLEscapeString(body) + "</pre>")
	}
	// goldmark выводит атрибуты в известном порядке — метку класса ставим здесь,
	// а не своим рендерером ссылок ради одного слова.
	return template.HTML(strings.ReplaceAll(buf.String(), `<a href="/doc/`, `<a class="wikilink" href="/doc/`))
}

// linkWikilinks заменяет [[цель|подпись]] на обычную markdown-ссылку.
// Код не трогаем: [[пример]] внутри ``` остаётся текстом — то же правило,
// по которому индексатор не считает такие примеры связями.
func linkWikilinks(body string, resolve func(string) string) string {
	var out strings.Builder
	pos := 0
	for _, seg := range codeSpans(body) {
		out.WriteString(replaceWikilinks(body[pos:seg[0]], resolve))
		out.WriteString(body[seg[0]:seg[1]])
		pos = seg[1]
	}
	out.WriteString(replaceWikilinks(body[pos:], resolve))
	return out.String()
}

// codeSpans — участки, которые разметкой не считаются: блоки ``` и `код`.
func codeSpans(body string) [][]int {
	spans := append(reFencedCode.FindAllStringIndex(body, -1), reInlineCode.FindAllStringIndex(body, -1)...)
	sort.Slice(spans, func(i, j int) bool { return spans[i][0] < spans[j][0] })
	merged := [][]int{}
	for _, s := range spans {
		// инлайн-код внутри блока не должен резать его надвое
		if n := len(merged); n > 0 && s[0] < merged[n-1][1] {
			if s[1] > merged[n-1][1] {
				merged[n-1][1] = s[1]
			}
			continue
		}
		merged = append(merged, []int{s[0], s[1]})
	}
	return merged
}

func replaceWikilinks(text string, resolve func(string) string) string {
	return reWikilink.ReplaceAllStringFunc(text, func(m string) string {
		sub := reWikilink.FindStringSubmatch(m)
		inner := strings.ReplaceAll(sub[1], `\|`, "|")
		parts := strings.Split(inner, "|")
		target := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(strings.Split(parts[0], "#")[0]), "/"), ".md")
		display := strings.TrimSpace(parts[0])
		if len(parts) > 1 {
			display = strings.TrimSpace(parts[1])
		}
		id := resolve(target)
		if id == "" {
			return m // битую ссылку не выдаём за рабочую; линтер её и не пропустит
		}
		// подпись экранируем от разметки ссылок, чтобы «]» не рвал её
		display = strings.NewReplacer("[", `\[`, "]", `\]`).Replace(display)
		return "[" + display + "](" + docURL(id) + ")"
	})
}
