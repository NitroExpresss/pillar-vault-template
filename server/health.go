// Здоровье хранилища в приложении: тот же линтер, что в CI и перед
// выкладкой, но для человека — без агента битую ссылку или карточку без
// summary чинит сам хозяин, и ему надо видеть, где именно.
package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

type healthIssue struct {
	File  string `json:"file"`
	ID    string `json:"id,omitempty"` // карточка, если файл — карточка хранилища
	Title string `json:"title,omitempty"`
	What  string `json:"what"`
}

type healthReport struct {
	Cards    int           `json:"cards"`
	Errors   []healthIssue `json:"errors"`
	Warnings []healthIssue `json:"warnings"`
}

// проверка обходит диск — повторяем её, только когда хранилище пересобрано
var health struct {
	sync.Mutex
	at  time.Time
	rep *healthReport
}

func healthOf(v *Vault) healthReport {
	health.Lock()
	defer health.Unlock()
	if health.rep != nil && health.at.Equal(v.Generated) {
		return *health.rep
	}
	rep := healthReport{Errors: []healthIssue{}, Warnings: []healthIssue{}}
	cards, errs, warns, err := lintCheck(root)
	if err != nil {
		rep.Errors = append(rep.Errors, healthIssue{What: err.Error()})
	}
	rep.Cards = cards
	issue := func(s string) healthIssue {
		file, what, ok := strings.Cut(s, ": ")
		if !ok {
			return healthIssue{What: s}
		}
		is := healthIssue{File: file, What: what}
		if d := v.Docs[strings.TrimSuffix(file, ".md")]; d != nil {
			is.ID, is.Title = d.ID, d.Title
		}
		return is
	}
	for _, e := range errs {
		rep.Errors = append(rep.Errors, issue(e))
	}
	for _, w := range warns {
		rep.Warnings = append(rep.Warnings, issue(w))
	}
	health.at, health.rep = v.Generated, &rep
	return rep
}

// handleHealthAPI — GET /api/health: ошибки и предупреждения линтера.
func handleHealthAPI(w http.ResponseWriter, r *http.Request) {
	if !widgetAuth(w, r) {
		return
	}
	json.NewEncoder(w).Encode(healthOf(currentVault()))
}
