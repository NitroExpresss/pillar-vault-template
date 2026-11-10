// Покупки: блок на дашборде из tasks/shopping.md. Это не дела на день,
// поэтому из «Учёта дел» и из выбора дел они убраны; здесь — по разделам
// карточки, с отметкой «куплено» и быстрым добавлением.
package main

import (
	"net/http"
	"strings"
	"unicode/utf8"
)

const shoppingCard = "tasks/shopping"

type shopGroup struct {
	Name string
	Open []taskItem
	Done []taskItem
}

type shopView struct {
	Groups   []shopGroup
	Sections []string // разделы карточки — куда класть новую покупку
	Open     int
	Done     int
}

// shopSections — заголовки «## …» карточки покупок, в порядке появления.
func shopSections(d *Doc) []string {
	out := []string{}
	for _, l := range strings.Split(normalizeEOL(d.Body), "\n") {
		if strings.HasPrefix(l, "## ") {
			out = append(out, strings.TrimSpace(strings.TrimPrefix(l, "## ")))
		}
	}
	return out
}

func shopping(v *Vault) *shopView {
	d := v.Docs[shoppingCard]
	if d == nil {
		return nil
	}
	sv := &shopView{Sections: shopSections(d)}
	if len(sv.Sections) == 0 {
		sv.Sections = []string{"Покупки"}
	}
	byName := map[string]int{}
	for _, s := range sv.Sections {
		byName[s] = len(sv.Groups)
		sv.Groups = append(sv.Groups, shopGroup{Name: s})
	}
	if v.Tasks != nil {
		for _, t := range v.Tasks.Items {
			if t.Src != shoppingCard {
				continue
			}
			i, ok := byName[t.Group]
			if !ok {
				byName[t.Group] = len(sv.Groups)
				i = len(sv.Groups)
				sv.Groups = append(sv.Groups, shopGroup{Name: t.Group})
			}
			if t.Done {
				sv.Groups[i].Done = append(sv.Groups[i].Done, t)
				sv.Done++
			} else {
				sv.Groups[i].Open = append(sv.Groups[i].Open, t)
				sv.Open++
			}
		}
	}
	return sv
}

// handleShop — «Купить…»: строка-чекбокс в выбранный раздел карточки.
func handleShop(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	fail := func(msg string) { http.Redirect(w, r, "/?m="+urlQuery(msg)+"#shopping", http.StatusSeeOther) }
	v := currentVault()
	d := v.Docs[shoppingCard]
	if d == nil {
		fail("нет карточки " + shoppingCard)
		return
	}
	text := strings.Join(strings.Fields(r.FormValue("text")), " ")
	switch {
	case text == "":
		fail("пустая покупка")
		return
	case utf8.RuneCountInString(text) > maxTaskRunes:
		fail("слишком длинно")
		return
	}
	group := r.FormValue("group")
	known := false
	for _, s := range shopSections(d) {
		known = known || s == group
	}
	if !known { // раздела нет (или карточка без разделов) — в первый или новый «Покупки»
		if ss := shopSections(d); len(ss) > 0 {
			group = ss[0]
		} else {
			group = "Покупки"
		}
	}
	if err := editCard(d.Path, "купить: "+text, func(raw string) (string, bool) {
		return insertInSection(raw, "## "+group, "- [ ] "+text), true
	}); err != nil {
		fail("не записано")
		return
	}
	http.Redirect(w, r, "/#shopping", http.StatusSeeOther)
}
