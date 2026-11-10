// Экраны React-приложения помимо дашборда: карточка, раздел, тема, поиск,
// правка, «Доступ». Те же данные, что у прежних страниц (handleDoc,
// handleFolder…), JSON-ом; разметку карточки по-прежнему разбирает сервер
// (goldmark) — в браузер уходит готовый HTML, как и раньше уходил страницей.
package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

type docRef struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	URL     string   `json:"url"`
	Summary string   `json:"summary,omitempty"`
	Type    string   `json:"type,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Folder  string   `json:"folder,omitempty"`
	Snippet string   `json:"snippet,omitempty"`
}

func refOf(d *Doc) docRef {
	return docRef{ID: d.ID, Title: d.Title, URL: docURL(d.ID), Summary: d.Summary, Type: d.Type, Tags: d.Tags}
}

func refsOf(list []*Doc) []docRef {
	out := make([]docRef, 0, len(list))
	for _, d := range list {
		out = append(out, refOf(d))
	}
	return out
}

type crumbJSON struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type docPage struct {
	docRef
	Status    string      `json:"status,omitempty"`
	Pin       bool        `json:"pin"`
	Path      string      `json:"path"`
	Words     int         `json:"words"`
	Created   string      `json:"created,omitempty"`
	Updated   string      `json:"updated,omitempty"`
	HTML      string      `json:"html"`
	Crumbs    []crumbJSON `json:"crumbs"`
	Links     []docRef    `json:"links"`
	Backlinks []docRef    `json:"backlinks"`
}

// handleDocAPI — GET /api/doc?id=…
func handleDocAPI(w http.ResponseWriter, r *http.Request) {
	if !widgetAuth(w, r) {
		return
	}
	v := currentVault()
	d := v.Docs[r.URL.Query().Get("id")]
	if d == nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"нет такой карточки"}`))
		return
	}
	out := docPage{docRef: refOf(d), Status: d.Status, Pin: isPinned(d), Path: d.Path, Words: d.Wordcount, Created: d.Created,
		Updated: fmString(d.FM, "updated"), HTML: string(docHTML(v, d)), Crumbs: []crumbJSON{},
		Links: refsOf(linksOf(v, d)), Backlinks: refsOf(backlinksOf(v, d))}
	for _, c := range docCrumbs(v, d) {
		out.Crumbs = append(out.Crumbs, crumbJSON{Label: c.Label, URL: c.URL})
	}
	json.NewEncoder(w).Encode(out)
}

type listPage struct {
	Title string   `json:"title"`
	Kind  string   `json:"kind"` // folder, tag, search
	Items []docRef `json:"items"`
	Kids  []navTag `json:"kids,omitempty"` // у темы — её ветки
}

// handleListAPI — GET /api/list?folder=… | ?tag=… | ?q=…
func handleListAPI(w http.ResponseWriter, r *http.Request) {
	if !widgetAuth(w, r) {
		return
	}
	v, q := currentVault(), r.URL.Query()
	var out listPage
	switch {
	case q.Has("q"):
		query := strings.TrimSpace(q.Get("q"))
		out = listPage{Title: query, Kind: "search", Items: []docRef{}}
		for _, h := range search(v, query) {
			ref := refOf(h.Doc)
			ref.Snippet, ref.Folder = h.Snippet, folderLabel(v, h.Doc.Folder)
			out.Items = append(out.Items, ref)
		}
	case q.Has("tag"):
		tag := q.Get("tag")
		list := []*Doc{}
		for _, d := range v.Index {
			for _, t := range d.Tags {
				if t == tag || strings.HasPrefix(t, tag+"/") {
					list = append(list, d)
					break
				}
			}
		}
		out = listPage{Title: tag, Kind: "tag", Items: refsOf(sortByTitle(list))}
		for t, n := range v.Tags {
			if strings.HasPrefix(t, tag+"/") {
				out.Kids = append(out.Kids, navTag{Name: t, Count: n, URL: tagURL(t)})
			}
		}
		sort.Slice(out.Kids, func(i, j int) bool { return out.Kids[i].Name < out.Kids[j].Name })
	default:
		path := q.Get("folder")
		if path == "" {
			path = "."
		}
		list := []*Doc{}
		for _, d := range v.Index {
			if d.Folder == path || strings.HasPrefix(d.Folder, path+"/") {
				list = append(list, d)
			}
		}
		out = listPage{Title: folderLabel(v, path), Kind: "folder", Items: refsOf(sortByTitle(list))}
	}
	for i := range out.Items {
		if out.Items[i].Snippet == "" {
			out.Items[i].Snippet = snippet(v.Docs[out.Items[i].ID])
		}
	}
	json.NewEncoder(w).Encode(out)
}

type cardRaw struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Path  string `json:"path"`
	Raw   string `json:"raw"`
	Ver   string `json:"ver"`
}

type cardSaved struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Ver     string `json:"ver,omitempty"`  // при конфликте — версия на диске: повторное сохранение пишет осознанно
	Disk    string `json:"disk,omitempty"` // при конфликте — что сейчас лежит на диске
}

// handleCardAPI — GET /api/card?id=… — исходник для редактора;
// POST /api/card (id, content, ver) — сохранить с защитой от перезаписи.
func handleCardAPI(w http.ResponseWriter, r *http.Request) {
	if !widgetAuth(w, r) {
		return
	}
	v := currentVault()
	if r.Method == http.MethodPost {
		if !wantsAPI(r) { // формы с чужого сайта без CORS-заголовка сюда не дойдут
			w.WriteHeader(http.StatusForbidden)
			return
		}
		r.ParseForm()
	}
	d := v.Docs[r.FormValue("id")]
	if d == nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"нет такой карточки"}`))
		return
	}
	raw, err := readCard(d.Path)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"карточка не читается"}`))
		return
	}
	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(cardRaw{ID: d.ID, Title: d.Title, Path: d.Path, Raw: raw, Ver: cardVersion(raw)})
		return
	}
	content := normalizeEOL(r.FormValue("content"))
	conflict, err := saveEdit(d, raw, content, r.FormValue("ver"))
	switch {
	case conflict:
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(cardSaved{Message: conflictNotice, Ver: cardVersion(raw), Disk: raw})
	case err != nil:
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(cardSaved{Message: "не сохранено: " + err.Error()})
	default:
		json.NewEncoder(w).Encode(cardSaved{OK: true, Ver: cardVersion(content)})
	}
}

type accessEntry struct {
	At     string `json:"at"`
	Result string `json:"result"`
	IP     string `json:"ip"`
	Device string `json:"device"`
	OK     bool   `json:"ok"`
}

// handleAccessAPI — GET /api/access: журнал входов и можно ли сменить ключ
// с сайта. Смена — прежней формой POST /access.
func handleAccessAPI(w http.ResponseWriter, r *http.Request) {
	if !widgetAuth(w, r) {
		return
	}
	out := struct {
		FromEnv bool          `json:"fromEnv"`
		Logins  []accessEntry `json:"logins"`
	}{FromEnv: hashFromEnv, Logins: []accessEntry{}}
	for _, e := range readLogins(loginShown) {
		out.Logins = append(out.Logins, accessEntry{At: e.At(), Result: e.Result, IP: e.IP, Device: e.Device(), OK: e.OK()})
	}
	json.NewEncoder(w).Encode(out)
}
