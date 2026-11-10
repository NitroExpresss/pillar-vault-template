package main

import (
	"strings"
	"testing"
	"time"
)

func TestНеделяГраницыИИмя(t *testing.T) {
	day := func(s string) time.Time { d, _ := time.Parse("2006-01-02", s); return d }
	cases := []struct{ in, mon, sun, id, span string }{
		{"2026-10-21", "2026-10-19", "2026-10-25", "2026-W43", "19–25 октября"},
		{"2026-10-25", "2026-10-19", "2026-10-25", "2026-W43", "19–25 октября"}, // воскресенье — ещё та же неделя
		{"2026-10-28", "2026-10-26", "2026-11-01", "2026-W44", "26 октября – 1 ноября"},
		{"2027-01-01", "2026-12-28", "2027-01-03", "2026-W53", "28 декабря – 3 января"}, // ISO-год на стыке
	}
	for _, c := range cases {
		mon, sun := weekBounds(day(c.in))
		if ymd(mon) != c.mon || ymd(sun) != c.sun || weekID(day(c.in)) != c.id || weekSpan(mon, sun) != c.span {
			t.Errorf("%s: %s..%s %s «%s», ожидалось %s..%s %s «%s»", c.in, ymd(mon), ymd(sun), weekID(day(c.in)),
				weekSpan(mon, sun), c.mon, c.sun, c.id, c.span)
		}
	}
}

func TestНедельнаяСводкаСчитаетСтолпыИДневник(t *testing.T) {
	dir := lintFixture(t, map[string]string{
		"pillars/daily-vows.md": "---\ntype: pillar\nsummary: с\ntags: [цели]\n---\n## Столпы\n\n- **Сон** — до 00\n\n## Журнал\n\n" +
			"| Дата | Сон |\n|---|---|\n| 2026-10-19 | ✓ |\n| 2026-10-20 | ✗ |\n| 2026-10-20 | ✗ |\n| 2026-10-12 | ✓ |\n",
		"notes/diary.md": "---\ntype: note\nsummary: д\ntags: [pkm]\n---\n## 2026-10-18 21:00\n\nпрошлая\n\n## 2026-10-20 22:00\n\nэта неделя\n",
	})
	v, err := buildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	now, _ := time.Parse("2006-01-02 15:04", "2026-10-21 12:00")
	out := weekReport(v, now)
	for _, want := range []string{
		"id: week-2026-w43", "tags: [цели, неделя]",
		"| Сон | 1 | 1 | 1 |",         // задвоенная строка за 20-е не считается дважды; 21-е ещё не отмечено
		"держал 1 из 3", "эта неделя", // запись этой недели есть,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("в сводке нет %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "прошлая") {
		t.Error("в сводку попала запись прошлой недели")
	}
}

func TestСферыКолесаСобираютЦелиИПересмотр(t *testing.T) {
	dir := lintFixture(t, map[string]string{
		"pillars/life-goals.md": "---\ntype: pillar\nsummary: ц\ntags: [цели]\nstatus: draft\nreviewed: 2026-07-01\n---\n" +
			"## 💪 Здоровье (6/10)\n- чекап\n\n## 💰 Финансы (3/10)\n- подушка\n",
		"goals/w.md":   "---\ntype: goal\nsummary: вес\ntags: [цели]\nstatus: draft\nsphere: Здоровье\n---\n# Вес\n",
		"goals/old.md": "---\ntype: goal\nsummary: старое\ntags: [цели]\nstatus: archived\nsphere: здоровье\n---\n# Старое\n",
	})
	v, err := buildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	var health *Sphere
	for i := range v.Spheres {
		if v.Spheres[i].Key == "здоровье" {
			health = &v.Spheres[i]
		}
	}
	if health == nil || len(health.Cards) != 1 || health.Cards[0].ID != "goals/w" {
		t.Fatalf("к «Здоровью» привязано не то (архив не в счёт): %#v", health)
	}
	now, _ := time.Parse("2006-01-02 15:04", "2026-10-21 12:00")
	out := weekReport(v, now)
	for _, want := range []string{"Без целей и планов: 💰 Финансы.", "Пересмотр целей просрочен:** последний 2026-07-01, срок был 2026-10-01"} {
		if !strings.Contains(out, want) {
			t.Errorf("в сводке нет %q:\n%s", want, out)
		}
	}
}
