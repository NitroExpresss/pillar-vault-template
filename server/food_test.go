// Журнал еды: запись приёма, сегодняшний день в JSON виджета, отмена записи.
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestЖурналЕды(t *testing.T) {
	dir := vaultFixture(t)
	head := "---\ntype: note\nsummary: \"еда\"\ntags: [здоровье]\nstatus: active\n---\n"
	os.WriteFile(filepath.Join(dir, "notes", "food.md"), []byte(head+"# Еда\n\n## Связи\n\n- нет\n\n---\n\n## 2026-01-01\n\n- 09:00 вчерашняя каша\n"), 0o644)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)

	post := func(form url.Values) {
		req := asAdmin(httptest.NewRequest(http.MethodPost, "/food", strings.NewReader(form.Encode())))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		handleFood(httptest.NewRecorder(), req)
	}
	today := func() widgetFood {
		rec := httptest.NewRecorder()
		handleFoodJSON(rec, asAdmin(httptest.NewRequest(http.MethodGet, "/food.json", nil)))
		var got widgetFood
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("не JSON: %v", err)
		}
		return got
	}

	rec := httptest.NewRecorder()
	handleFoodJSON(rec, httptest.NewRequest(http.MethodGet, "/food.json", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("гостю отдана еда: %d", rec.Code)
	}
	if got := today(); len(got.Items) != 0 || got.Date != ymd(time.Now()) {
		t.Fatalf("сегодня пусто, а пришло: %+v", got)
	}

	post(url.Values{"text": {" гречка  с курицей "}})
	post(url.Values{"text": {"яблоко"}})
	got := today()
	if len(got.Items) != 2 || got.Items[0].Text != "гречка с курицей" || got.Items[1].Text != "яблоко" || got.Items[0].Time == "" {
		t.Fatalf("сегодня: %+v", got)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "notes", "food.md"))
	if !strings.Contains(string(raw), "- 09:00 вчерашняя каша\n\n## "+ymd(time.Now())+"\n\n- ") {
		t.Fatalf("новый день не в конце:\n%s", raw)
	}

	post(url.Values{"do": {"remove"}, "time": {got.Items[0].Time}, "text": {"гречка с курицей"}})
	if got := today(); len(got.Items) != 1 || got.Items[0].Text != "яблоко" {
		t.Fatalf("после отмены: %+v", got)
	}
}

func TestЕдаПоследниеПриёмыИИстория(t *testing.T) {
	body := "# Еда\n\n## 2026-01-01\n\n- 08:00 каша\n- 19:00 ужин\n\n## 2026-01-03\n\n- 12:00 суп\n\n## 2026-01-02\n\n## 2026-01-04\n\n- 09:00 сегодня\n"
	days := foodDays(body)
	if len(days) != 3 || days[0].Date != "2026-01-04" || days[2].Date != "2026-01-01" {
		t.Fatalf("дни: %+v", days)
	}
	// за двое суток до 4-го: 3-е и 2-е (пустое — не в журнале), 1-е — уже старше
	got := foodRecentDays(days, "2026-01-04", 2)
	if len(got) != 1 || got[0].Date != "2026-01-03" || got[0].Items[0].Text != "суп" {
		t.Fatalf("последние дни: %+v", got)
	}
	if got := foodRecentDays(days, "2026-01-04", 3); len(got) != 2 || got[1].Date != "2026-01-01" {
		t.Fatalf("три дня: %+v", got)
	}
}

func TestЕдаДописываетсяВКонецДня(t *testing.T) {
	raw := "# Еда\n\n## 2026-01-02\n\n- 08:00 каша\n\n## 2026-01-03\n\n- 09:00 сыр\n"
	got := foodAdd(raw, "2026-01-02", "- 13:00 суп")
	if !strings.Contains(got, "- 08:00 каша\n- 13:00 суп\n\n## 2026-01-03") {
		t.Fatalf("не в конец дня:\n%s", got)
	}
	items := foodDay(got+"- без времени\n", "2026-01-03")
	if len(items) != 2 || items[1] != (foodItem{Text: "без времени"}) {
		t.Fatalf("день: %+v", items)
	}
}

func TestВыборПротоколаПишетсяВЖурнал(t *testing.T) {
	dir := vaultFixture(t)
	head := "---\ntype: note\nsummary: \"еда\"\ntags: [здоровье]\nstatus: active\nfast_goal: 36\n---\n"
	os.WriteFile(filepath.Join(dir, "notes", "food.md"), []byte(head+"# Еда\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "guides"), 0o755)
	os.WriteFile(filepath.Join(dir, "guides", "intermittent-fasting-keto.md"), []byte("---\ntype: guide\nsummary: \"гайд\"\ntags: [здоровье]\nstatus: complete\n---\n# Гайд\n\n## Протоколы\n\n| Протокол | Голод, ч | Окно еды, ч |\n|---|---|---|\n| 16:8 | 16 | 8 |\n"), 0o644)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)

	post := func(form url.Values) {
		req := asAdmin(httptest.NewRequest(http.MethodPost, "/fast", strings.NewReader(form.Encode())))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		handleFast(httptest.NewRecorder(), req)
	}
	post(url.Values{"protocol": {"16:8"}, "window": {"12:00"}})
	v := fastNow(currentVault(), time.Now())
	if v.Protocol != "16:8" || v.Goal != 16 || v.Plan == nil || v.Plan.Start != "12:00" || len(v.Protocols) != 1 {
		t.Fatalf("после выбора: %+v", v)
	}
	post(url.Values{"protocol": {"выдуманный"}})
	if v := fastNow(currentVault(), time.Now()); v.Protocol != "16:8" {
		t.Fatal("принят протокол, которого нет в гайде")
	}
	post(url.Values{"protocol": {""}, "goal": {"42"}})
	v = fastNow(currentVault(), time.Now())
	raw, _ := os.ReadFile(filepath.Join(dir, "notes", "food.md"))
	if v.Protocol != "" || v.Goal != 42 || v.Plan != nil || strings.Contains(string(raw), "fast_window") {
		t.Fatalf("своя цель: %+v\n%s", v, raw)
	}
}
