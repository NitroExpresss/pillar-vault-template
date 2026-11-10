// Голодание по журналу еды: окна еды, прошлые голодания, вехи текущего.
package main

import (
	"testing"
	"time"
)

func TestГолоданиеПоЖурналуЕды(t *testing.T) {
	body := "# Еда\n\n## Связи\n\n- 10:00 не еда, а связь\n\n---\n\n" +
		"## 2026-01-01\n\n- 12:00 завтрак\n- 13:30 добавка\n- 19:00 ужин\n\n" +
		"## 2026-01-03\n\n- 9:00 каша\n- без времени\n"
	ms := meals(body)
	if len(ms) != 4 || ms[3].Format("2006-01-02 15:04") != "2026-01-03 09:00" {
		t.Fatalf("приёмы: %v", ms)
	}
	now := time.Date(2026, 1, 4, 3, 0, 0, 0, time.Local) // 18 ч после каши
	v := fasting(ms, 36, now)
	if v.Hours != 18 || v.Pct != 50 || v.Stage != "кетоз" || v.Next != "аутофагия" || v.NextIn != 6 {
		t.Fatalf("сейчас: %+v", v)
	}
	// 12:00 и 13:30 — одно окно, 19:00 — другое (дальше трёх часов)
	kinds := ""
	for _, w := range v.Windows {
		kinds += w.Kind + " "
	}
	if kinds != "eat fast eat fast eat now " {
		t.Fatalf("окна: %q", kinds)
	}
	if len(v.Fasts) != 2 || v.Fasts[0].Hours != 38 || v.Fasts[1].Hours != 5.5 || v.Best != 38 {
		t.Fatalf("прошлые: %+v", v.Fasts)
	}
	if e := fasting(nil, 36, now); e.Last != "" || e.Hours != 0 || len(e.Windows) != 0 {
		t.Fatalf("без еды: %+v", e)
	}
}

func TestПротоколыГолоданияИзГайда(t *testing.T) {
	guide := &Doc{Body: "# Гайд\n\n## Протоколы\n\n| Протокол | Голод, ч | Окно еды, ч | Суть |\n|---|---|---|---|\n" +
		"| 16:8 | 16 | 8 | база |\n| OMAD | 23 | 1 | один приём |\n| 36 ч | 36 | — | ADF |\n\n## Дальше\n\n| x | 1 | 2 |\n"}
	ps := fastProtocols(guide)
	if len(ps) != 3 || ps[0] != (fastProto{"16:8", 16, 8, "база"}) || ps[2].Window != 0 || ps[2].ID != "36 ч" {
		t.Fatalf("протоколы: %+v", ps)
	}
	food := &Doc{FM: map[string]any{"fast_protocol": "16:8", "fast_window": "12:00", "fast_goal": "40"}}
	if p, goal, w := fastSetup(food, ps); p == nil || p.ID != "16:8" || goal != 16 || w != "12:00" {
		t.Fatalf("выбор: %+v %v %q", p, goal, w)
	}
	food.FM["fast_protocol"] = "чужой"
	if p, goal, _ := fastSetup(food, ps); p != nil || goal != 40 {
		t.Fatalf("без протокола — своя цель: %+v %v", p, goal)
	}

	at := func(hm string) time.Time {
		t, _ := time.ParseInLocation("2006-01-02 15:04", "2026-01-05 "+hm, time.Local)
		return t
	}
	if w := fastWindow("12:00", 8, at("10:30")); w == nil || w.Open || w.In != 1.5 || w.End != "20:00" {
		t.Fatalf("до окна: %+v", w)
	}
	if w := fastWindow("12:00", 8, at("19:00")); !w.Open || w.In != 1 {
		t.Fatalf("в окне: %+v", w)
	}
	if w := fastWindow("12:00", 8, at("21:00")); w.Open || w.In != 15 {
		t.Fatalf("после окна: %+v", w)
	}
	// окно 20:00–02:00 через полночь: в 01:00 оно ещё открыто
	if w := fastWindow("20:00", 6, at("01:00")); !w.Open || w.In != 1 {
		t.Fatalf("через полночь: %+v", w)
	}
	if fastWindow("", 8, at("10:00")) != nil || fastWindow("12:00", 0, at("10:00")) != nil {
		t.Fatal("окно без времени или без длины")
	}
}
