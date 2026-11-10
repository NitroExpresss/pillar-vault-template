// Виджеты «Столпы» и «Покупки»: только по сессии, и ровно то, что видно на
// дашборде.
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

func TestВиджетСтолпов(t *testing.T) {
	dir := vaultFixture(t)
	yesterday := ymd(time.Now().AddDate(0, 0, -1))
	os.WriteFile(filepath.Join(dir, "pillars", "daily-vows.md"), []byte("---\ntype: pillar\nsummary: \"с\"\ntags: [цели]\nstatus: complete\n---\n"+
		"## Столпы\n\n- **Сон** — до полуночи\n- **Вода** — два литра\n\n"+
		"## Журнал\n\n| Дата | Сон | Вода |\n|---|---|---|\n| "+yesterday+" | ✓ | ✗ |\n"), 0o644)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)

	rec := httptest.NewRecorder()
	handleVowsJSON(rec, httptest.NewRequest(http.MethodGet, "/vows.json", nil))
	if rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "Сон") {
		t.Fatalf("гостю отданы столпы: %d", rec.Code)
	}
	get := func() widgetVows {
		rec := httptest.NewRecorder()
		handleVowsJSON(rec, asAdmin(httptest.NewRequest(http.MethodGet, "/vows.json", nil)))
		var got widgetVows
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("не JSON: %v", err)
		}
		return got
	}
	got := get()
	if len(got.Vows) != 2 || got.Vows[0].Name != "Сон" || got.Vows[0].Rule != "до полуночи" ||
		got.Vows[0].Streak != 1 || got.Vows[1].Streak != 0 || got.Held != 0 {
		t.Fatalf("столпы: %+v", got)
	}

	// отметка с виджета — тот же POST /vow, что на дашборде
	form := url.Values{"name": {"Сон"}, "mark": {vowHeld}}
	req := asAdmin(httptest.NewRequest(http.MethodPost, "/vow", strings.NewReader(form.Encode())))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handleVow(httptest.NewRecorder(), req)
	if got := get(); got.Vows[0].Mark != vowHeld || got.Vows[0].Streak != 2 || got.Held != 1 {
		t.Fatalf("отметка не видна виджету: %+v", got.Vows[0])
	}
}

func TestВиджетПокупок(t *testing.T) {
	dir := vaultFixture(t)
	head := "---\ntype: todo\nsummary: \"x\"\ntags: [дела]\nstatus: draft\n---\n"
	os.MkdirAll(filepath.Join(dir, "tasks"), 0o755)
	os.WriteFile(filepath.Join(dir, "tasks", "shopping.md"), []byte(head+"# Покупки\n\n## Еда\n\n- [ ] Хлеб\n- [x] Молоко\n\n## Быт\n\n- [ ] Мыло\n"), 0o644)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)

	rec := httptest.NewRecorder()
	handleShopJSON(rec, httptest.NewRequest(http.MethodGet, "/shop.json", nil))
	if rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "Хлеб") {
		t.Fatalf("гостю отданы покупки: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	handleShopJSON(rec, asAdmin(httptest.NewRequest(http.MethodGet, "/shop.json", nil)))
	var got widgetShop
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("не JSON: %v", err)
	}
	if got.Src != shoppingCard || len(got.Items) != 2 || got.Items[0] != (widgetShopItem{"Хлеб", "Еда"}) ||
		got.Items[1].Group != "Быт" || got.Done != 1 || len(got.Sections) != 2 {
		t.Fatalf("покупки: %+v", got)
	}
}

func TestВиджетСегодняПомечаетПросрочку(t *testing.T) {
	dir := vaultFixture(t)
	head := "---\ntype: todo\nsummary: \"x\"\ntags: [дела]\nstatus: draft\n---\n"
	os.MkdirAll(filepath.Join(dir, "tasks"), 0o755)
	far := ymd(time.Now().AddDate(0, 0, 30))
	os.WriteFile(filepath.Join(dir, "tasks", "day.md"), []byte(head+"## Текущий день\n\n- [ ] Старое @2020-01-01\n- [ ] Новое @"+far+"\n- [ ] Без срока\n"), 0o644)
	hash, _ := hashPassword("ключ")
	withVault(t, dir, hash)
	rec := httptest.NewRecorder()
	handleTodayJSON(rec, asAdmin(httptest.NewRequest(http.MethodGet, "/today.json", nil)))
	var got widgetToday
	json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Tasks) != 3 || !got.Tasks[0].Late || got.Tasks[1].Late || got.Tasks[2].Late {
		t.Fatalf("просрочка: %+v", got.Tasks)
	}
}
