// API для React-приложения (web/, план plans/react-migration).
//
// Записи идут через те же обработчики форм, что у сайта: приложение шлёт
// форму с заголовком X-Pillar-Api, а apiForms превращает редирект-ответ
// обработчика в JSON. Логика правки карточек не дублируется — её по-прежнему
// держат handleToday, handleVow и остальные.
//
// Чтения — по экрану, а не выгрузкой хранилища: в браузер уходит то же, что
// ушло бы страницей. Всё — только по сессии и с no-store.
package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	// apiHeader ставит приложение на каждый запрос. Нестандартный заголовок
	// чужой сайт без CORS-разрешения прислать не может — это и защита от
	// подделки формы, и признак «ответь JSON, а не редиректом».
	apiHeader = "X-Pillar-Api"
	// okHeader — обработчик отвечает успехом, хотя в редиректе есть ?m=:
	// «записано», «витрина обновлена». Без него m в ответе значит отказ.
	okHeader = "X-Pillar-Ok"
	// spaDir — собранное приложение (web/dist): сюда его кладёт deploy.sh
	// из ветки web-dist, а при разработке — npm run build и копия.
	spaDir = "viewer/x"
)

func wantsAPI(r *http.Request) bool { return r.Header.Get(apiHeader) != "" }

// markOK — успех, который несёт сообщение в ?m= (см. okHeader).
func markOK(w http.ResponseWriter) { w.Header().Set(okHeader, "1") }

type apiResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// apiWriter глотает редирект обработчика и запоминает, куда тот вёл.
type apiWriter struct {
	http.ResponseWriter
	redirected bool
	location   string
}

func (a *apiWriter) WriteHeader(code int) {
	if code >= 300 && code < 400 {
		a.redirected = true
		a.location = a.Header().Get("Location")
		return
	}
	a.ResponseWriter.WriteHeader(code)
}

func (a *apiWriter) Write(b []byte) (int, error) {
	if a.redirected {
		return len(b), nil // тело редиректа приложению не нужно
	}
	return a.ResponseWriter.Write(b)
}

// apiForms — POST от приложения: редирект обработчика → {"ok","message"}.
// Ошибки обработчика (400, 401, 405) проходят как есть, с его текстом.
func apiForms(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !wantsAPI(r) || r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		aw := &apiWriter{ResponseWriter: w}
		next.ServeHTTP(aw, r)
		if !aw.redirected {
			return
		}
		res := apiResult{OK: true}
		if u, err := url.Parse(aw.location); err == nil {
			res.Message = u.Query().Get("m")
		}
		if res.Message != "" && w.Header().Get(okHeader) == "" {
			res.OK = false
		}
		h := w.Header()
		h.Del("Location")
		h.Del(okHeader)
		h.Set("Content-Type", "application/json; charset=utf-8")
		h.Set("Cache-Control", "private, no-store")
		if !res.OK {
			w.WriteHeader(http.StatusUnprocessableEntity)
		}
		json.NewEncoder(w).Encode(res)
	})
}

// ─────────────────── навигация: разделы и темы ───────────────────

type navDir struct {
	Path  string   `json:"path"`
	Label string   `json:"label"`
	Count int      `json:"count"`
	URL   string   `json:"url"`
	Dirs  []navDir `json:"dirs"`
	Files []navDoc `json:"files"`
}

type navDoc struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

type navTag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Share int    `json:"share"` // доля темы, % — шкала в меню
	URL   string `json:"url"`
}

type navData struct {
	Dirs   []navDir `json:"dirs"`
	Files  []navDoc `json:"files"`
	Tags   []navTag `json:"tags"`
	Cards  int      `json:"cards"`
	Inbox  int      `json:"inbox"`  // мыслей ждёт разбора
	Broken int      `json:"broken"` // ошибок линтера: «Здоровье» в меню
}

func navLevel(tv treeView) ([]navDir, []navDoc) {
	dirs, files := []navDir{}, []navDoc{}
	for _, d := range tv.Dirs {
		sub, subFiles := navLevel(d.Child)
		dirs = append(dirs, navDir{Path: d.Node.Path, Label: d.Label, Count: d.Count,
			URL: folderURL(d.Node.Path), Dirs: sub, Files: subFiles})
	}
	for _, f := range tv.Files {
		files = append(files, navDoc{ID: f.Doc.ID, Title: f.Doc.Title, URL: docURL(f.Doc.ID)})
	}
	return dirs, files
}

// handleNav — меню приложения: то же дерево и те же темы, что в сайдбаре.
func handleNav(w http.ResponseWriter, r *http.Request) {
	if !widgetAuth(w, r) {
		return
	}
	v := currentVault()
	out := navData{Tags: []navTag{}, Cards: len(v.Index), Inbox: inboxCount(v) + len(inboxOf(v).Files), Broken: len(healthOf(v).Errors)}
	out.Dirs, out.Files = navLevel(treeRoot(v, "", ""))
	for _, t := range topTags(v) {
		out.Tags = append(out.Tags, navTag{Name: t.Name, Count: t.Count, Share: t.Share, URL: tagURL(t.Name)})
	}
	json.NewEncoder(w).Encode(out)
}

// ─────────────────── само приложение ───────────────────

// serveApp — файл сборки приложения (viewer/x, её кладёт deploy.sh из ветки
// web-dist). Вызывается только по сессии (handleRoot). Файлы с хэшем в
// имени не меняются — их можно держать; index.html всегда сверяется, иначе
// после выкладки браузер грузил бы старую сборку.
func serveApp(w http.ResponseWriter, r *http.Request, rel string) {
	base := filepath.Join(root, filepath.FromSlash(spaDir))
	full := filepath.Join(base, filepath.FromSlash(path.Clean("/"+rel)))
	if st, err := os.Stat(full); err != nil || st.IsDir() {
		if rel != "index.html" {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "приложение не собрано: npm run build в web/ или ветка web-dist", http.StatusServiceUnavailable)
		return
	}
	if strings.HasPrefix(rel, "assets/") {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "private, no-cache")
	}
	w.Header().Set("Vary", "Cookie")
	f, err := os.Open(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	// ServeContent, а не ServeFile: тот на путь, кончающийся на index.html, отвечает редиректом
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}
