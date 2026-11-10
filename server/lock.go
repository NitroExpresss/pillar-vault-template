// Форма входа — единственная страница, которую рисует Go. Всё остальное —
// приложение (web/), а гостю за этой формой не отдаётся ничего.
//
// Шаблон лежит файлом в viewer/templates/ и перечитывается на лету
// (watchVault): правка вида формы не требует пересборки бинаря.
package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

var (
	lockTmpl     atomic.Pointer[template.Template]
	tmplStampVal atomic.Value // string
)

func templatesDir() string { return filepath.Join(root, "viewer", "templates") }

// loadTemplates разбирает шаблон заново. Ошибка не заменяет уже работающий:
// сломанная правка не должна запирать вход.
func loadTemplates() error {
	t, err := template.ParseFiles(filepath.Join(templatesDir(), "lock.html"))
	if err != nil {
		return fmt.Errorf("шаблон формы входа: %w", err)
	}
	lockTmpl.Store(t)
	tmplStampVal.Store(tmplStamp())
	return nil
}

// tmplStamp — отпечаток каталога шаблонов: имена и время правки.
func tmplStamp() string {
	entries, err := os.ReadDir(templatesDir())
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, e := range entries {
		if info, err := e.Info(); err == nil {
			fmt.Fprintf(&b, "%s:%d;", e.Name(), info.ModTime().UnixNano())
		}
	}
	return b.String()
}

type lockData struct {
	Notice string // «ключ не подошёл», «слишком много попыток»
	Next   string // куда вернуться после ключа (только свой путь)
}

func renderLock(w http.ResponseWriter, r *http.Request) {
	t := lockTmpl.Load()
	if t == nil {
		http.Error(w, "форма входа не загружена", http.StatusInternalServerError)
		return
	}
	next := r.URL.Query().Get("next")
	if localBack(next) != next {
		next = "" // форма входа не должна уводить на чужой сайт
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Vary", "Cookie")
	if err := t.Execute(w, lockData{Notice: r.URL.Query().Get("m"), Next: next}); err != nil {
		log.Printf("форма входа: %v", err) // заголовки ушли — 500 писать поздно
	}
}
