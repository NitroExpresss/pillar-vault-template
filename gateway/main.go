// Pillar Gateway — локальный шлюз к LLM-провайдерам.
//
// Зачем: юнит pillar-vault.service закрыт на петлю (IPAddressDeny=any) —
// хранилище наружу ходить не может и не должно. Этот крошечный процесс —
// единственная дверь: слушает только петлю, держит ключи провайдеров в
// gateway/providers.json (0600, вне git) и пересылает OpenAI-совместимые
// запросы (/v1/models, /v1/chat/completions) апстриму.
//
// Конфиг перечитывается при изменении файла: смена ключа перезапуска
// не требует.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// ─────────────────── конфиг ───────────────────

type gwProvider struct {
	BaseURL string   `json:"base_url"`
	Key     string   `json:"key"`
	Models  []string `json:"models"`
}

type gwConfig struct {
	Default   string                `json:"default"`
	Providers map[string]gwProvider `json:"providers"`
}

var (
	cfgPath  string
	cfgMu    sync.Mutex
	cfg      gwConfig
	cfgStamp time.Time
)

// loadConfig читает конфиг; перечитывает файл, только если тот изменился.
func loadConfig() (gwConfig, error) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	st, err := os.Stat(cfgPath)
	if err != nil {
		return gwConfig{}, fmt.Errorf("конфиг: %w", err)
	}
	if cfg.Providers != nil && st.ModTime().Equal(cfgStamp) {
		return cfg, nil
	}
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return gwConfig{}, fmt.Errorf("конфиг: %w", err)
	}
	var c gwConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return gwConfig{}, fmt.Errorf("конфиг битый: %w", err)
	}
	cfg, cfgStamp = c, st.ModTime()
	return c, nil
}

// resolveProvider находит провайдера под модель: «имя/модель» — по имени
// провайдера, иначе по списку моделей, иначе провайдер по умолчанию.
func resolveProvider(c gwConfig, model string) (gwProvider, string, error) {
	if i := strings.Index(model, "/"); i > 0 {
		if p, ok := c.Providers[model[:i]]; ok {
			return p, model[i+1:], nil
		}
	}
	for _, p := range c.Providers {
		for _, m := range p.Models {
			if m == model {
				return p, model, nil
			}
		}
	}
	if p, ok := c.Providers[c.Default]; ok {
		return p, model, nil
	}
	return gwProvider{}, model, fmt.Errorf("нет провайдера под модель %q", model)
}

// ─────────────────── HTTP ───────────────────

var upstream = &http.Client{Timeout: 120 * time.Second}

func gwErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"message": msg, "type": "gateway_error"},
	})
}

// handleModels — список моделей из конфига (апстрим не дёргаем).
func handleModels(w http.ResponseWriter, r *http.Request) {
	c, err := loadConfig()
	if err != nil {
		gwErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	type modelObj struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	}
	out := struct {
		Object string     `json:"object"`
		Data   []modelObj `json:"data"`
	}{Object: "list", Data: []modelObj{}}
	names := make([]string, 0, len(c.Providers))
	for name := range c.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, m := range c.Providers[name].Models {
			out.Data = append(out.Data, modelObj{ID: m, Object: "model", OwnedBy: name})
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(out)
}

// handleChat — пересылка chat/completions апстриму с ключом провайдера.
// Ответ (включая stream) отдаётся как есть, со статусом апстрима.
func handleChat(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		gwErr(w, http.StatusBadRequest, "не прочитать запрос")
		return
	}
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		gwErr(w, http.StatusBadRequest, "запрос не JSON")
		return
	}
	model, _ := req["model"].(string)
	if model == "" {
		gwErr(w, http.StatusBadRequest, "в запросе нет model")
		return
	}
	c, err := loadConfig()
	if err != nil {
		gwErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	p, bare, err := resolveProvider(c, model)
	if err != nil {
		gwErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req["model"] = bare
	up, _ := json.Marshal(req)

	r2, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		strings.TrimRight(p.BaseURL, "/")+"/chat/completions", bytes.NewReader(up))
	if err != nil {
		gwErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	r2.Header.Set("Content-Type", "application/json")
	if p.Key != "" {
		r2.Header.Set("Authorization", "Bearer "+p.Key)
	}
	resp, err := upstream.Do(r2)
	if err != nil {
		gwErr(w, http.StatusBadGateway, "апстрим: "+err.Error())
		return
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18789", "адрес (только петля!)")
	flag.StringVar(&cfgPath, "config", "providers.json", "файл провайдеров")
	flag.Parse()
	if !strings.HasPrefix(*addr, "127.0.0.1") && !strings.HasPrefix(*addr, "localhost") {
		log.Fatal("шлюз слушает только петлю; наружу выставлять нельзя")
	}
	if _, err := loadConfig(); err != nil {
		log.Printf("%v — подхвачу при появлении файла", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", handleModels)
	mux.HandleFunc("POST /v1/chat/completions", handleChat)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	log.Printf("pillar-gateway слушает %s, конфиг %s", *addr, cfgPath)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
