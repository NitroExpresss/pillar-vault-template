// Правка цели без markdown: показания, срок и новые результаты — формами
// из виджета «Цели». Таблица «Ключевые результаты» переписывается целиком
// по разобранным ячейкам; остальная карточка не трогается.
package main

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// заготовка новой цели (newcard.go): колонка без цели, её заменит первый результат
const goalPlaceholder = "Результат, ед"

// krTable — таблица результатов: колонки, строка «Цель», строки с датами.
type krTable struct {
	cols   []string
	target []string
	rows   [][]string // [дата, значения…]
}

// editGoalTable разбирает таблицу «Ключевые результаты», даёт её изменить и
// пишет обратно на то же место; раздела нет — заводит его в конце.
func editGoalTable(raw string, edit func(t *krTable) error) (string, error) {
	lines := strings.Split(normalizeEOL(raw), "\n")
	head, from, to := -1, -1, -1 // заголовок раздела, первая и последняя+1 строки таблицы
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "## ") {
			if head >= 0 {
				break
			}
			if t == goalKRHeading {
				head = i
			}
			continue
		}
		if head < 0 {
			continue
		}
		if strings.HasPrefix(t, "|") {
			if from < 0 {
				from = i
			}
			to = i + 1
		} else if from >= 0 && t != "" {
			break
		}
	}
	tb := &krTable{cols: []string{"Дата"}}
	if from >= 0 {
		for _, l := range lines[from:to] {
			cells := []string{}
			for _, c := range strings.Split(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(l), "|"), "|"), "|") {
				cells = append(cells, strings.TrimSpace(c))
			}
			rule := true
			for _, c := range cells {
				rule = rule && reTableRule.MatchString(c)
			}
			switch {
			case rule:
			case len(tb.cols) == 1 && tb.target == nil && len(tb.rows) == 0 && !strings.EqualFold(cells[0], "Цель") && !reISODate.MatchString(cells[0]):
				tb.cols = cells
			case strings.EqualFold(cells[0], "Цель"):
				tb.target = cells[1:]
			case reISODate.MatchString(cells[0]):
				tb.rows = append(tb.rows, cells)
			}
		}
	}
	if err := edit(tb); err != nil {
		return "", err
	}
	// выравниваем ширину и сортируем показания по дате
	n := len(tb.cols)
	pad := func(c []string, w int) []string {
		for len(c) < w {
			c = append(c, "")
		}
		return c[:w]
	}
	sort.SliceStable(tb.rows, func(i, j int) bool { return tb.rows[i][0] < tb.rows[j][0] })
	var b []string
	row := func(c []string) string { return "| " + strings.Join(c, " | ") + " |" }
	b = append(b, row(tb.cols))
	rule := make([]string, n)
	for i := range rule {
		rule[i] = "---"
	}
	b = append(b, "|"+strings.Join(rule, "|")+"|")
	b = append(b, row(append([]string{"Цель"}, pad(tb.target, n-1)...)))
	for _, r := range tb.rows {
		b = append(b, row(pad(r, n)))
	}
	switch {
	case from >= 0:
		lines = append(lines[:from], append(b, lines[to:]...)...)
	case head >= 0:
		lines = append(lines[:head+1], append(append([]string{""}, b...), lines[head+1:]...)...)
	default:
		lines = append(lines, "", goalKRHeading, "")
		lines = append(lines, b...)
	}
	return strings.Join(lines, "\n"), nil
}

// handleGoal — POST /goal: id и do —
// value (date, v<i> по колонкам: пустое — не меняли), deadline (deadline:
// дата или пусто — снять), kr (name, unit, target — новый результат).
func handleGoal(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	v := currentVault()
	d := v.Docs[r.FormValue("id")]
	fail := func(msg string) { http.Redirect(w, r, urlWithMsg("/", msg), http.StatusSeeOther) }
	if d == nil || d.Type != "goal" {
		fail("нет такой цели")
		return
	}
	var edit func(string) (string, error)
	what := ""
	switch r.FormValue("do") {
	case "deadline":
		dl := strings.TrimSpace(r.FormValue("deadline"))
		if dl != "" {
			if _, err := time.Parse("2006-01-02", dl); err != nil || !reISODate.MatchString(dl) {
				fail("срок — дата ГГГГ-ММ-ДД")
				return
			}
		}
		what = "срок цели: " + dl
		edit = func(raw string) (string, error) { return setFM(raw, "deadline", dl) }
	case "value":
		date := r.FormValue("date")
		if _, err := time.Parse("2006-01-02", date); err != nil || !reISODate.MatchString(date) {
			fail("нет такой даты")
			return
		}
		what = "показания цели за " + date
		edit = func(raw string) (string, error) {
			return editGoalTable(raw, func(t *krTable) error {
				vals := make([]string, len(t.cols)-1)
				any := false
				for i := range vals {
					s := strings.TrimSpace(r.FormValue(fmt.Sprintf("v%d", i)))
					if s == "" {
						continue
					}
					if parseNum(s) == nil {
						return fmt.Errorf("«%s» — не число", s)
					}
					vals[i] = strings.ReplaceAll(s, ",", ".")
					any = true
				}
				if !any {
					return errors.New("впишите хотя бы одно показание")
				}
				for k := range t.rows {
					if t.rows[k][0] != date {
						continue
					}
					// та же дата — дописываем в ту же строку
					row := t.rows[k]
					for len(row) < len(t.cols) {
						row = append(row, "")
					}
					for i, s := range vals {
						if s != "" {
							row[i+1] = s
						}
					}
					t.rows[k] = row
					return nil
				}
				t.rows = append(t.rows, append([]string{date}, vals...))
				return nil
			})
		}
	case "kr":
		name := strings.Join(strings.Fields(r.FormValue("name")), " ")
		unit := strings.Join(strings.Fields(r.FormValue("unit")), " ")
		target := strings.TrimSpace(r.FormValue("target"))
		switch {
		case name == "" || utf8.RuneCountInString(name) > 40 || strings.ContainsAny(name+unit, "|,"):
			fail("название результата — до 40 знаков, без «|» и запятых")
			return
		case parseNum(target) == nil:
			fail("цель результата — число")
			return
		}
		col := name
		if unit != "" {
			col += ", " + unit
		}
		what = "результат цели: " + col
		edit = func(raw string) (string, error) {
			return editGoalTable(raw, func(t *krTable) error {
				for i, c := range t.cols {
					if strings.EqualFold(c, col) {
						return errors.New("такой результат уже есть")
					}
					// заготовка без цели и показаний — заменяем, а не копим пустые колонки
					if c == goalPlaceholder && (i-1 >= len(t.target) || t.target[i-1] == "") {
						t.cols[i] = col
						for len(t.target) < len(t.cols)-1 {
							t.target = append(t.target, "")
						}
						t.target[i-1] = strings.ReplaceAll(target, ",", ".")
						return nil
					}
				}
				for len(t.target) < len(t.cols)-1 {
					t.target = append(t.target, "")
				}
				t.cols = append(t.cols, col)
				t.target = append(t.target, strings.ReplaceAll(target, ",", "."))
				return nil
			})
		}
	default:
		http.Error(w, "неизвестное действие", http.StatusBadRequest)
		return
	}
	var bad error
	if err := editCard(d.Path, what, func(raw string) (string, bool) {
		next, err := edit(raw)
		bad = err
		return next, err == nil
	}); err != nil {
		if bad != nil {
			err = bad
		}
		fail(err.Error())
		return
	}
	http.Redirect(w, r, "/#goals", http.StatusSeeOther)
}
