// Недельная сводка — факты недели одной карточкой для регулярной операции
// агента (SOUL.md, «Регулярные операции»).
//
//	pillar-vault -root . -week                  # неделя, в которую входит сегодня
//	pillar-vault -root . -week -date 2026-10-25 # неделя, в которую входит дата
//
// Считает сервер, а не агент: столпы, дневник и замеры разбираются теми же
// функциями, что рисуют дашборд, — цифры в сводке и на панели не разойдутся.
// Агенту остаётся раздел «Разбор»: что из этих цифр следует.
package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// weekCardDir — где лежат сводки; имя карточки — ISO-неделя: 2026-W43.
const weekCardDir = "notes/weekly"

// weekBounds — понедельник и воскресенье ISO-недели, в которую входит day.
func weekBounds(day time.Time) (mon, sun time.Time) {
	d := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	back := (int(d.Weekday()) + 6) % 7 // пн → 0, вс → 6
	mon = d.AddDate(0, 0, -back)
	return mon, mon.AddDate(0, 0, 6)
}

// weekID — «2026-W43»: ISO-год может не совпадать с календарным на стыке лет.
func weekID(day time.Time) string {
	y, w := day.ISOWeek()
	return fmt.Sprintf("%d-W%02d", y, w)
}

// weekSpan — «19–25 октября» или «27 октября – 2 ноября».
func weekSpan(mon, sun time.Time) string {
	if mon.Month() == sun.Month() {
		return fmt.Sprintf("%d–%d %s", mon.Day(), sun.Day(), ruMonths[sun.Month()-1])
	}
	return fmt.Sprintf("%d %s – %d %s", mon.Day(), ruMonths[mon.Month()-1], sun.Day(), ruMonths[sun.Month()-1])
}

// reviewEvery — пересмотр целей раз в квартал (life-goals, `review:`).
const reviewEvery = 3 // месяца

// reviewLine — когда пересматривать цели: `reviewed:` в шапке life-goals
// плюс квартал. Без даты пересмотра строки нет — считать не от чего.
func reviewLine(v *Vault, now time.Time) (line string, overdue bool) {
	d := v.Docs[goalsCard]
	if d == nil {
		return "", false
	}
	last, err := time.ParseInLocation("2006-01-02", fmString(d.FM, "reviewed"), now.Location())
	if err != nil {
		return "", false
	}
	next := ymd(last.AddDate(0, reviewEvery, 0))
	if ymd(now) >= next {
		return fmt.Sprintf("**Пересмотр целей просрочен:** последний %s, срок был %s — [[%s]].", ymd(last), next, goalsCard), true
	}
	return fmt.Sprintf("Следующий пересмотр целей — %s.", next), false
}

// weekReport собирает карточку недели. now — момент отчёта: дни после него
// ещё не прожиты и в «без отметки» не считаются.
func weekReport(v *Vault, now time.Time) string {
	mon, sun := weekBounds(now)
	from, to := ymd(mon), ymd(sun)
	lastLived := to
	if today := ymd(now); today < lastLived {
		lastLived = today
	}
	lived := daysBetween(from, lastLived) + 1
	inWeek := func(date string) bool { return date >= from && date <= to }

	var body strings.Builder
	var brief []string // для summary: самое важное одной строкой

	// ── столпы ──
	body.WriteString("## Столпы\n\n")
	if len(v.Vows) == 0 || v.VowLog == nil {
		body.WriteString("Столпов или журнала нет — [[" + vowsCard + "]].\n\n")
	} else {
		body.WriteString("| Столп | Держал | Порвал | Без отметки | Стрик |\n|---|---|---|---|---|\n")
		held, broke := 0, 0
		for _, vow := range v.Vows {
			h, b := 0, 0
			seen := map[string]bool{} // строка за дату могла задвоиться при ручной правке
			for _, row := range v.VowLog.Rows {
				if len(row) == 0 || !inWeek(row[0]) || row[0] > lastLived || seen[row[0]] {
					continue
				}
				seen[row[0]] = true
				switch v.VowLog.marksOn(row[0])[vow.Name] {
				case vowHeld:
					h++
				case vowBroke:
					b++
				}
			}
			cur, rec, _ := v.VowLog.streakOf(vow.Name, now)
			fmt.Fprintf(&body, "| %s | %d | %d | %d | %d (рекорд %d) |\n", vow.Name, h, b, lived-h-b, cur, rec)
			held += h
			broke += b
		}
		total := lived * len(v.Vows)
		fmt.Fprintf(&body, "\nИтого: держал %d из %d отметок недели, порвал %d. Журнал — [[%s]].\n\n", held, total, broke, vowsCard)
		brief = append(brief, fmt.Sprintf("столпы %d/%d", held, total))
	}

	// ── тело ──
	body.WriteString("## Тело\n\n")
	if m := v.Metrics; m == nil || len(m.Rows) == 0 {
		body.WriteString("Замеров нет.\n\n")
	} else {
		// база — последний замер до недели, конец — последний внутри неё
		var base, last *metricRow
		for i := range m.Rows {
			r := &m.Rows[i]
			if r.Date < from {
				base = r
			} else if inWeek(r.Date) {
				if base == nil && last == nil {
					base = r
				}
				last = r
			}
		}
		if last == nil {
			body.WriteString("За неделю замеров не было.\n\n")
			brief = append(brief, "без замеров")
		} else {
			if m.Demo {
				body.WriteString("> Демо-данные: карточка замеров ещё с `demo: true`.\n\n")
			}
			fmt.Fprintf(&body, "| Показатель | %s | %s | Δ |\n|---|---|---|---|\n", base.Date, last.Date)
			for i, col := range m.Columns {
				a, b := cell(base, i), cell(last, i)
				if a == nil || b == nil {
					continue
				}
				fmt.Fprintf(&body, "| %s | %s | %s | %s |\n", col, num(*a), num(*b), signed(*b-*a))
				if i == 0 && base != last {
					brief = append(brief, fmt.Sprintf("%s %s (%s)", strings.ToLower(col), num(*b), signed(*b-*a)))
				}
			}
			if g := v.Goal; g != nil {
				fmt.Fprintf(&body, "\nДо цели %s: %s, пройдено %d%%.\n", num(g.Target), num(g.Left), g.Pct)
			}
			fmt.Fprintf(&body, "\nКарточка — [[%s]].\n\n", m.Source)
		}
	}

	// ── дневник ──
	body.WriteString("## Дневник\n\n")
	entries := []DiaryEntry{}
	if d := v.Docs[diaryCard]; d != nil {
		for _, e := range diaryEntries(d.Body) {
			if len(e.When) >= 10 && inWeek(e.When[:10]) {
				entries = append(entries, e)
			}
		}
	}
	if len(entries) == 0 {
		body.WriteString("Записей за неделю нет.\n\n")
	} else {
		for _, e := range entries {
			fmt.Fprintf(&body, "- **%s** — %s\n", e.When, clip(strings.Join(strings.Fields(e.Text), " "), 140))
		}
		fmt.Fprintf(&body, "\nЦеликом — [[%s]].\n\n", diaryCard)
	}
	brief = append(brief, fmt.Sprintf("дневник %d", len(entries)))

	// ── сроки ──
	body.WriteString("## Сроки\n\n")
	today, horizon := ymd(now), ymd(now.AddDate(0, 0, 7))
	var over, next []taskItem
	if v.Tasks != nil {
		for _, t := range v.Tasks.Items {
			if t.Done || t.Due == "" {
				continue
			}
			if t.Due < today {
				over = append(over, t)
			} else if t.Due <= horizon {
				next = append(next, t)
			}
		}
	}
	if len(over)+len(next) == 0 {
		body.WriteString("Ни просрочки, ни сроков на неделю вперёд.\n\n")
	}
	for _, part := range []struct {
		title string
		items []taskItem
	}{{"Просрочено", over}, {"На неделю вперёд", next}} {
		if len(part.items) == 0 {
			continue
		}
		fmt.Fprintf(&body, "**%s:**\n\n", part.title)
		for _, t := range part.items {
			fmt.Fprintf(&body, "- %s — до %s · [[%s]]\n", t.Text, t.Due, t.Src)
		}
		body.WriteString("\n")
	}
	if len(over) > 0 {
		brief = append(brief, fmt.Sprintf("просрочено %d", len(over)))
	}

	// ── колесо ──
	body.WriteString("## Колесо баланса\n\n")
	var low []string
	for _, s := range v.Spheres {
		if s.Low {
			low = append(low, fmt.Sprintf("%s (%d/%d)", s.Name, s.Score, s.Max))
		}
	}
	switch {
	case len(v.Spheres) == 0:
		body.WriteString("Сфер нет — [[" + goalsCard + "]].\n\n")
	case len(low) == 0:
		body.WriteString("Провисающих сфер нет.\n\n")
	default:
		fmt.Fprintf(&body, "Провисают: %s. Оценки — [[%s]].\n\n", strings.Join(low, ", "), goalsCard)
	}
	var bare []string // сферы, которым не отвечает ни одна цель, план или проект
	for _, s := range v.Spheres {
		if len(s.Cards) == 0 {
			bare = append(bare, s.Name)
		}
	}
	if len(bare) > 0 {
		fmt.Fprintf(&body, "Без целей и планов: %s.\n\n", strings.Join(bare, ", "))
	}
	if line, overdue := reviewLine(v, now); line != "" {
		body.WriteString(line + "\n\n")
		if overdue {
			brief = append(brief, "пересмотр целей просрочен")
		}
	}

	body.WriteString("## Разбор\n\n" +
		"<!-- пишет агент: что получилось, что провисло и почему, одно решение на следующую неделю -->\n")

	id, span := weekID(now), weekSpan(mon, sun)
	summary := fmt.Sprintf("Неделя %s (%s): %s", id, span, strings.Join(brief, ", "))
	return "---\n" +
		"id: week-" + strings.ToLower(id) + "\n" +
		"type: note\n" +
		"summary: \"" + strings.ReplaceAll(summary, `"`, `'`) + "\"\n" +
		"tags: [цели, неделя]\n" +
		"status: draft\n" +
		"created: " + ymd(now) + "\n" +
		"---\n\n" +
		"# Неделя " + id + " · " + span + "\n\n" +
		"> Факты собрал `pillar-vault -week` из карточек; разбор — ниже, словами.\n\n" +
		body.String()
}

func cell(r *metricRow, i int) *float64 {
	if r == nil || i >= len(r.Values) {
		return nil
	}
	return r.Values[i]
}

func signed(f float64) string {
	f = math.Round(f*10) / 10
	switch {
	case f > 0:
		return "+" + num(f)
	case f < 0:
		return "−" + num(-f)
	}
	return "0"
}

// clip — по буквам, а не байтам: кириллица не режется посреди символа.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}
