// Ввод замеров тела с дашборда и из виджета: строка в таблицу карточки
// с `metrics: body`, а не в отдельную базу. Источник истины — markdown.
package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// metricsDoc — карточка замеров (та, по которой строится панель «Тело»).
func metricsDoc(v *Vault) *Doc {
	if v.Metrics != nil {
		return v.Docs[v.Metrics.Source]
	}
	for _, d := range v.Index {
		if fmString(d.FM, "metrics") == "body" {
			return d
		}
	}
	return nil
}

// handleMetric: do=add — строка замера за дату (по умолчанию сегодня; та же
// дата — строка заменяется); do=cleardemo — убрать демо-строки и флаг.
// Значения — числа: запятая или точка, пустое — колонку не меняли.
func handleMetric(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	back := localBack(r.FormValue("back"))
	if back == "/" {
		back = "/#metrics"
	}
	fail := func(msg string) {
		if i := strings.IndexByte(back, '#'); i >= 0 {
			sep := "?"
			if strings.Contains(back[:i], "?") {
				sep = "&"
			}
			back = back[:i] + sep + "m=" + urlQuery(msg) + back[i:]
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
	}
	v := currentVault()
	d := metricsDoc(v)
	if d == nil {
		fail("нет карточки с metrics: body")
		return
	}

	if r.FormValue("do") == "cleardemo" {
		if err := editCard(d.Path, "замеры: демо-строки убраны", clearDemo); err != nil {
			fail("демо не убрано: " + err.Error())
			return
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}

	date := strings.TrimSpace(r.FormValue("date"))
	if date == "" {
		date = ymd(time.Now())
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		fail("нет такой даты")
		return
	}
	raw, err := readCard(d.Path)
	if err != nil {
		fail("карточка замеров не читается")
		return
	}
	header := metricHeader(raw)
	if len(header) < 2 {
		fail("в карточке нет таблицы замеров")
		return
	}
	cells, any := make([]string, len(header)-1), false
	for i := range cells {
		s := strings.ReplaceAll(strings.TrimSpace(r.FormValue("v"+strconv.Itoa(i))), ",", ".")
		if s == "" {
			continue
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || f <= 0 || f > 1000 {
			fail("«" + header[i+1] + "»: нужно число")
			return
		}
		cells[i], any = strconv.FormatFloat(f, 'f', 1, 64), true
	}
	if !any {
		fail("пустой замер")
		return
	}
	if err := editCard(d.Path, "замер "+date, func(raw string) (string, bool) {
		return upsertMetricRow(raw, date, cells)
	}); err != nil {
		fail("замер не записан")
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// metricTable — границы первой таблицы карточки: шапка, разделитель, строки.
func metricTable(lines []string) (head, end int) {
	head = -1
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "|") {
			head = i
			break
		}
	}
	if head < 0 || head+1 >= len(lines) {
		return -1, -1
	}
	end = head + 2
	for end < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[end]), "|") {
		end++
	}
	return head, end
}

func metricHeader(raw string) []string {
	lines := strings.Split(normalizeEOL(raw), "\n")
	head, _ := metricTable(lines)
	if head < 0 {
		return nil
	}
	return tableCells(lines[head])
}

// upsertMetricRow — строка за дату: есть — заменяется (пустые ячейки
// берутся из прежней строки), нет — встаёт на своё место по дате.
func upsertMetricRow(raw, date string, cells []string) (string, bool) {
	lines := strings.Split(normalizeEOL(raw), "\n")
	head, end := metricTable(lines)
	if head < 0 {
		return raw, false
	}
	row := func(vals []string) string { return "| " + date + " | " + strings.Join(vals, " | ") + " |" }
	at := end
	for i := head + 2; i < end; i++ {
		c := tableCells(lines[i])
		if len(c) == 0 {
			continue
		}
		if c[0] == date {
			merged := append([]string{}, cells...)
			for j := range merged {
				if merged[j] == "" && j+1 < len(c) {
					merged[j] = c[j+1]
				}
			}
			lines[i] = row(merged)
			return strings.Join(lines, "\n"), true
		}
		if reISODate.MatchString(c[0]) && c[0] > date && at == end {
			at = i
		}
	}
	out := append(append(append([]string{}, lines[:at]...), row(cells)), lines[at:]...)
	return strings.Join(out, "\n"), true
}

// clearDemo — убрать выдуманные строки: остаётся первый замер (в карточке
// он настоящий — старт плана), уходят флаг demo и предупреждение о демо.
func clearDemo(raw string) (string, bool) {
	lines := strings.Split(normalizeEOL(raw), "\n")
	head, end := metricTable(lines)
	if head < 0 {
		return raw, false
	}
	out := []string{}
	firstKept, inWarn := false, false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case t == "demo: true":
			continue
		case i >= head+2 && i < end:
			if firstKept {
				continue
			}
			firstKept = true
		case strings.HasPrefix(t, ">") && (inWarn || strings.Contains(t, "демонстрацион")):
			inWarn = true
			continue
		default:
			if inWarn && t == "" && len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
				inWarn = false
				continue // пустая строка после убранного предупреждения
			}
			inWarn = false
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n"), true
}

type metricField struct {
	Idx  int
	Name string
	Last string // последнее непустое значение — подсказкой в поле
}

// metricFields — поля формы «+ замер»: по колонке таблицы после даты.
func metricFields(m *Metrics) []metricField {
	out := []metricField{}
	if m == nil {
		return out
	}
	for i := 1; i < len(m.Columns); i++ {
		f := metricField{Idx: i - 1, Name: m.Columns[i]}
		for k := len(m.Rows) - 1; k >= 0 && f.Last == ""; k-- {
			if i-1 < len(m.Rows[k].Values) && m.Rows[k].Values[i-1] != nil {
				f.Last = num(*m.Rows[k].Values[i-1])
			}
		}
		out = append(out, f)
	}
	return out
}

func urlQuery(s string) string { return strings.TrimPrefix(urlWithMsg("", s), "?m=") }
