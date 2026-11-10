// Перенос дела на другой день — с виджетов «Что горит» и «Сегодня», без
// приложения. Срок живёт хвостом «@ГГГГ-ММ-ДД» в строке дела (parseTaskLine):
// перенос меняет или дописывает этот хвост там, где дело живёт.
package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// setTaskDue ставит срок date первому открытому делу с этим текстом.
func setTaskDue(raw, text, date string) (string, bool) {
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
		if t, _, done, ok := parseTaskLine(l); ok && !done && t == text {
			s := strings.TrimRight(l, " \t")
			if j := strings.LastIndex(s, "@"); j != -1 && reISODate.MatchString(s[j+1:]) {
				s = strings.TrimRight(s[:j], " \t")
			}
			lines[i] = s + " @" + date
			return strings.Join(lines, "\n"), true
		}
	}
	return raw, false
}

func tomorrow() string { return ymd(time.Now().AddDate(0, 0, 1)) }

// moveTask — срок дела src/text на date; копия в «Сегодня», если есть,
// уходит из дня: дело больше не на сегодня.
func moveTask(v *Vault, src, text, date string) string {
	d := v.Docs[src]
	if d == nil {
		return "такого дела нет"
	}
	if err := editCard(d.Path, "на "+date+": "+text, func(raw string) (string, bool) { return setTaskDue(raw, text, date) }); err != nil {
		return "срок не перенесён"
	}
	if day := v.Docs[dayCard]; day != nil && src != dayCard && hasTask(todayList(v), text) {
		if err := editCard(day.Path, "перенесено, из дня: "+text, func(raw string) (string, bool) { return removeTodayTask(raw, text) }); err != nil {
			return "срок перенесён, но дело осталось в дне"
		}
	}
	return ""
}

// laterFromDay — «на завтра» у дела из «Сегодня». Пришло из карточки — там
// ставится срок; своё дело дня уезжает в бэклог «По датам» на завтра.
func laterFromDay(v *Vault, text string) string {
	date := tomorrow()
	if home := taskHome(v, taskItem{Text: text, Src: dayCard}); home != dayCard {
		return moveTask(v, home, text, date)
	}
	plan := v.Docs[planCard]
	if plan == nil {
		return "нет карточки " + planCard
	}
	if err := editCard(plan.Path, "на "+date+": "+text, func(raw string) (string, bool) {
		return insertInSection(raw, planSection, "- [ ] "+text+" @"+date), true
	}); err != nil {
		return "дело не перенесено"
	}
	day := v.Docs[dayCard]
	if err := editCard(day.Path, "перенесено, из дня: "+text, func(raw string) (string, bool) { return removeTodayTask(raw, text) }); err != nil {
		return "перенесено, но осталось в дне"
	}
	return ""
}

type widgetBurn struct {
	Zones []boardZone `json:"zones"`
}

// handleBurnJSON — GET /burn.json: виджет «Что горит» — те же зоны, что на главной.
func handleBurnJSON(w http.ResponseWriter, r *http.Request) {
	if !widgetAuth(w, r) {
		return
	}
	json.NewEncoder(w).Encode(widgetBurn{Zones: boardZones(currentVault(), ymd(time.Now()))})
}

// boardZones — сроки зонами для дашборда и виджета.
func boardZones(v *Vault, today string) []boardZone {
	out := []boardZone{}
	for _, z := range burnZones(v, today) {
		bz := boardZone{Key: z.Key, Title: z.Title, Items: []boardDue{}}
		for _, t := range z.Items {
			bz.Items = append(bz.Items, boardDue{boardTask: toBoardTask(t), When: dueLabel(t.Due, today)})
		}
		out = append(out, bz)
	}
	return out
}
