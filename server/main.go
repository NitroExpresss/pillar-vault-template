// Pillar Vault — сервер хранилища.
//
// Задача одна и важная: карточки не покидают сервер без пароля. Страницы
// собираются здесь и уходят готовым HTML — в браузер не отдаётся ни модель
// хранилища, ни тела карточек. Без сессии видна только форма входа.
//
// Запуск из корня репозитория:
//
//	go run ./server -root .              # пароль возьмётся из server/.passhash
//	go run ./server -root . -hash        # сгенерировать хэш пароля
//	go run ./server -root . -lint        # проверить хранилище и выйти
//
// Пароль: PILLAR_PASS_HASH в окружении, иначе файл server/.passhash.
// Если не задан ни один — вход невозможен, и хранилище не отдаётся никому.
package main

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

const (
	sessionCookie = "pv_session"
	sessionTTL    = 7 * 24 * time.Hour
	pbkdf2Iters   = 600_000 // OWASP-минимум для PBKDF2-HMAC-SHA256
	maxAttempts   = 10      // попыток входа с одного адреса
	attemptWindow = 15 * time.Minute
	maxTrackedIPs = 1024 // после этого числа адресов карта попыток выметается
	maxSaveBytes  = 2 << 20
)

var (
	addr      = flag.String("addr", ":8080", "адрес прослушивания")
	rootDir   = flag.String("root", "..", "корень репозитория хранилища")
	makeHash  = flag.Bool("hash", false, "сгенерировать хэш пароля и выйти")
	checkPass = flag.Bool("check", false, "проверить пароль против настроенного хэша и выйти")
	lintRun   = flag.Bool("lint", false, "проверить хранилище (ссылки, id, summary, теги) и выйти")
	weekRun   = flag.Bool("week", false, "напечатать карточку недельной сводки (notes/weekly) и выйти")
	weekDate  = flag.String("date", "", "день недели для -week, ГГГГ-ММ-ДД (по умолчанию сегодня)")

	root     string
	attempts = newAttemptStore()
)

func main() {
	flag.Parse()

	if *makeHash {
		if err := printHash(); err != nil {
			log.Fatal(err)
		}
		return
	}

	var err error
	if root, err = filepath.Abs(*rootDir); err != nil {
		log.Fatalf("корень хранилища: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "SOUL.md")); err != nil {
		log.Fatalf("в %s не видно SOUL.md — это не корень хранилища", root)
	}

	if *lintRun { // проверка хранилища: карточки, ссылки, теги
		if !lintVault(root) {
			os.Exit(1)
		}
		return
	}

	if *weekRun { // факты недели карточкой
		now := time.Now()
		if *weekDate != "" {
			d, err := time.ParseInLocation("2006-01-02", *weekDate, time.Local)
			if err != nil {
				log.Fatalf("-date: нужна дата ГГГГ-ММ-ДД, а не %q", *weekDate)
			}
			// день из прошлого прожит целиком — отметки за него все в счёт
			now = d.Add(23*time.Hour + 59*time.Minute)
		}
		v, err := buildIndex(root)
		if err != nil {
			log.Fatalf("сборка модели: %v", err)
		}
		fmt.Print(weekReport(v, now))
		return
	}

	h := strings.TrimSpace(os.Getenv("PILLAR_PASS_HASH"))
	hashFromEnv = h != ""
	if !hashFromEnv {
		h = passHashFromFile()
		hashFileStamp = passHashStamp()
	}
	if h == "" {
		log.Print("пароль не задан (PILLAR_PASS_HASH или server/.passhash): вход отключён, хранилище не отдаётся никому")
	} else if _, err := verifyPassword("", h); err != nil {
		// формат хэша битый — честно гасим замок, а не отвергаем верный пароль
		log.Printf("хэш пароля не читается (%v): вход отключён, перегенерируй -hash", err)
	} else {
		setPassHash(h)
		log.Print("замок включён: вход по паролю")
	}

	if *checkPass { // проверить пароль против настроенного хэша, не поднимая сервер
		if err := checkPassword(); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := rebuild(); err != nil {
		log.Printf("сборка индекса: %v", err)
	}
	initAutoCommit()

	if err := loadTemplates(); err != nil {
		log.Fatalf("шаблоны страниц: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRoot) // приложение (по сессии), форма входа, статика
	mux.HandleFunc("/vow", handleVow)
	mux.HandleFunc("/diary", handleDiary)
	mux.HandleFunc("/today", handleToday)
	mux.HandleFunc("/today.json", handleTodayJSON)       // виджет «Сегодня»
	mux.HandleFunc("/calendar.json", handleCalendarJSON) // виджет «Календарь»
	mux.HandleFunc("/body.json", handleBodyJSON)         // виджет «Тело»
	mux.HandleFunc("/vows.json", handleVowsJSON)         // виджет «Столпы»
	mux.HandleFunc("/shop.json", handleShopJSON)         // виджет «Покупки»
	mux.HandleFunc("/pins.json", handlePinsJSON)         // виджет «Важное»
	mux.HandleFunc("/burn.json", handleBurnJSON)         // виджет «Что горит»
	mux.HandleFunc("/day.json", handleDayJSON)           // окно дня виджета «Календарь»
	mux.HandleFunc("/goals.json", handleGoalsJSON)       // виджет «Цели»
	mux.HandleFunc("/food.json", handleFoodJSON)         // виджет «Еда»
	mux.HandleFunc("/fast.json", handleFastJSON)         // виджет «Голодание»
	mux.HandleFunc("/pin", handlePin)
	mux.HandleFunc("/archive", handleArchive) // в архив и обратно
	mux.HandleFunc("/goal", handleGoal)       // цель без markdown: показание, срок, результат
	mux.HandleFunc("/capture", handleCapture) // мысль с виджета — в inbox
	mux.HandleFunc("/task", handleTask)
	mux.HandleFunc("/plan", handlePlan)
	mux.HandleFunc("/metric", handleMetric)
	mux.HandleFunc("/shop", handleShop)
	mux.HandleFunc("/food", handleFood) // что съел — в журнал еды
	mux.HandleFunc("/fast", handleFast) // протокол голодания и окно еды
	mux.HandleFunc("/gallery", handleGallery)
	mux.HandleFunc("/login", handleLogin)
	mux.HandleFunc("/logout", handleLogout)
	mux.HandleFunc("/access", handleAccess)        // журнал входов и смена пароля
	mux.HandleFunc("/app/", handleApp)             // свежий APK для кнопки «Обновить приложение»
	mux.HandleFunc("/api/nav", handleNav)          // меню приложения: разделы и темы
	mux.HandleFunc("/api/board", handleBoard)      // данные дашборда приложения
	mux.HandleFunc("/api/doc", handleDocAPI)       // карточка: разметка, связи, показания
	mux.HandleFunc("/api/list", handleListAPI)     // раздел, тема, поиск
	mux.HandleFunc("/api/card", handleCardAPI)     // исходник карточки и сохранение правки
	mux.HandleFunc("/api/access", handleAccessAPI) // журнал входов
	mux.HandleFunc("/api/new", handleNewAPI)       // новая карточка из приложения
	mux.HandleFunc("/api/inbox", handleInboxAPI)   // входящие: мысли из «Захвата»
	mux.HandleFunc("/api/health", handleHealthAPI) // здоровье хранилища: линтер для человека
	mux.HandleFunc("/inbox", handleInbox)          // разбор входящих (GET — экран приложения)

	// Подключения к LLM-провайдерам: экран, добавление, проверка
	// (server/connections.go).
	mux.HandleFunc("/api/connections", handleConnectionsAPI)
	mux.HandleFunc("/connections", handleConnections)

	// Разговор с моделью через подключения (server/ai.go): чат из
	// приложения и разбор входящих.
	mux.HandleFunc("/api/ai/chat", handleAIChat)
	mux.HandleFunc("/api/ai/inbox", handleAIInbox)

	go watchVault() // карточки подхватываются с диска — индекс держит сервер

	listenAddr := *addr
	if p := os.Getenv("PORT"); p != "" && !isFlagSet("addr") { // PORT уважаем: так запускают превью и хостинги
		listenAddr = ":" + p
	}
	shown := listenAddr
	if strings.HasPrefix(shown, ":") { // ":8080" читается как localhost:8080
		shown = "localhost" + shown
	}
	log.Printf("Pillar Vault → http://%s (корень: %s)", shown, root)
	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           logging(renewSession(apiForms(mux))),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}

// ─────────────────────────── вход ───────────────────────────

// Вход — обычная форма. Ключ уходит POST-ом, сессия ставится кукой,
// дальше редирект: обновление страницы не переотправляет пароль.
func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	// куда вернуться после ключа: в новый интерфейс (/x/) или в прежний
	next := localBack(r.FormValue("next"))
	gate := func(msg string) {
		u := urlWithMsg("/", msg)
		if next != "/" {
			u += "&next=" + url.QueryEscape(next)
		}
		http.Redirect(w, r, u, http.StatusSeeOther)
	}
	hash := passHash()
	if hash == "" {
		gate("вход отключён: пароль на сервере не задан")
		return
	}
	ip := clientIP(r)
	if !attempts.allow(ip) {
		logLogin(r, loginBlocked)
		gate("слишком много попыток, подождите")
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "битая форма", http.StatusBadRequest)
		return
	}
	ok, err := verifyPassword(r.FormValue("password"), hash)
	if err != nil {
		log.Printf("проверка пароля: %v", err)
		gate("сервер не смог проверить ключ")
		return
	}
	if !ok {
		attempts.fail(ip)
		logLogin(r, loginDenied)
		time.Sleep(400 * time.Millisecond) // ровная задержка: не даём мерить время ответа
		gate("ключ не подошёл")
		return
	}

	attempts.reset(ip)
	logLogin(r, loginOK)
	setSessionCookie(w, r)
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func setSessionCookie(w http.ResponseWriter, r *http.Request) {
	token, expires := newSession()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
	})
}

// renewSession продлевает живую сессию, пока ею пользуются: не чаще раза в
// сутки выдаёт новый билет на полные sessionTTL. Без этого вход истекал
// ровно через неделю после ввода пароля — и приложение, и виджеты падали
// посреди работы. Истёкший билет не продлевается: нужен пароль. Отозвать
// всё разом по-прежнему можно только сменой пароля.
func renewSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookie); err == nil && validSession(c.Value) {
			if exp, ok := sessionExpiry(c.Value); ok && time.Until(exp) < sessionTTL-24*time.Hour {
				setSessionCookie(w, r)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// sessionExpiry — срок из билета «до-когда.подпись» (подпись проверяет validSession).
func sessionExpiry(token string) (time.Time, bool) {
	dot := strings.IndexByte(token, '.')
	if dot <= 0 {
		return time.Time{}, false
	}
	exp, err := strconv.ParseInt(token[:dot], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(exp, 0), true
}

// Выход — снятие куки: билет подписан, а не записан, отзывать на сервере
// нечего. Отобрать доступ у чужого устройства — сменой пароля.
func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ─────────────────────────── статика ───────────────────────────

// handleRoot — всё, что не API и не формы. Экраны приложения отдают его
// index.html (маршруты разбирает оно само), гостю — форма входа с обратным
// адресом. /x/… — прежний адрес нового интерфейса: на нём стартовали
// APK до 1.0.14 и остались закладки, поэтому он ведёт на тот же путь в корне.
func handleRoot(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	switch {
	case p == "/x" || strings.HasPrefix(p, "/x/"):
		to := "/" + strings.TrimPrefix(strings.TrimPrefix(p, "/x"), "/")
		if r.URL.RawQuery != "" {
			to += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, to, http.StatusMovedPermanently)
	case isAppPath(p):
		if isAdmin(r) {
			serveApp(w, r, "index.html")
		} else if p == "/" {
			renderLock(w, r)
		} else {
			http.Redirect(w, r, "/?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		}
	case strings.HasPrefix(p, "/assets/"):
		if !isAdmin(r) {
			http.NotFound(w, r) // файлы приложения гостю не нужны: за формой входа пусто
			return
		}
		serveApp(w, r, strings.TrimPrefix(p, "/"))
	default:
		handleStatic(w, r)
	}
}

// isAppPath — экраны приложения (web/src/main.tsx, маршруты).
func isAppPath(p string) bool {
	if p == "/" || p == "/search" || p == "/access" || p == "/new" || p == "/inbox" || p == "/health" || p == "/connections" || p == "/ai" {
		return true
	}
	for _, pre := range []string{"/doc/", "/folder/", "/tag/", "/edit/"} {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	return false
}

// статика viewer/ — без листинга каталогов.
// URL чистим слэшевым path: filepath.Clean на Windows вернул бы "\\".
func handleStatic(w http.ResponseWriter, r *http.Request) {
	clean := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
	// Шаблоны страниц — исходники интерфейса, а не его файлы: наружу не отдаём.
	if strings.HasPrefix(clean, "/templates/") {
		http.NotFound(w, r)
		return
	}
	// Закрытому хранилищу для формы входа нужны стили, шрифты и скрипт.
	// Всё остальное в viewer/ — галерея ориентиров, обои — личное.
	if !isAdmin(r) && !isGateAsset(clean) {
		http.NotFound(w, r)
		return
	}
	full := filepath.Join(root, "viewer", filepath.FromSlash(clean))
	rel, err := filepath.Rel(filepath.Join(root, "viewer"), full)
	if err != nil || strings.HasPrefix(rel, "..") {
		http.NotFound(w, r)
		return
	}
	st, err := os.Stat(full)
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	// интерфейс всегда сверяется с сервером: иначе браузер держит старый
	// app.js и правки не видны (ответ 304 стоит дёшево)
	switch strings.ToLower(filepath.Ext(full)) {
	case ".html", ".js", ".css", ".json":
		w.Header().Set("Cache-Control", "no-cache")
	case ".woff2":
		// шрифт с таким имени уже не изменится — незачем и спрашивать
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	// Именно ServeContent, а не ServeFile: последний на запрос вида
	// /index.html отвечает редиректом, хотя файл открыт и готов к отдаче.
	http.ServeContent(w, r, full, st.ModTime(), f)
}

// isGateAsset — то немногое, чем рисуется форма входа: стили, шрифт и
// скрипт. Список закрытый: новый файл в viewer/ по умолчанию приватен,
// а не публичен по недосмотру.
func isGateAsset(clean string) bool {
	switch clean {
	case "/gate.css", "/gate.js", "/fonts.css", "/favicon.ico":
		return true
	}
	return strings.HasPrefix(clean, "/fonts/")
}

// ─────────────────────────── карточки на диске ───────────────────────────

func readCard(rel string) (string, error) {
	full, err := cardPath(rel)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// saveCard пишет карточку, коммитит её (если включён автокоммит) и сразу
// пересобирает модель: следующая страница должна показывать записанное.
// what — что сделано, для сообщения коммита.
func saveCard(rel, content, what string) error {
	full, err := cardPath(rel)
	if err != nil {
		return err
	}
	if _, err := os.Stat(full); err != nil { // правим только существующие карточки
		return errors.New("карточка не найдена")
	}
	if len(content) > maxSaveBytes {
		return errors.New("слишком большая карточка")
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return err
	}
	commitFiles(what, rel)
	return rebuild()
}

// ─────────────────────────── доступ ───────────────────────────

func isAdmin(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	return validSession(c.Value)
}

// cardPath проверяет, что путь ведёт к карточке внутри хранилища.
func cardPath(rel string) (string, error) {
	if rel == "" || !strings.HasSuffix(strings.ToLower(rel), ".md") {
		return "", errors.New("недопустимый путь")
	}
	full := filepath.Join(root, filepath.FromSlash(filepath.Clean("/"+rel)))
	inside, err := filepath.Rel(root, full)
	if err != nil || strings.HasPrefix(inside, "..") {
		return "", errors.New("путь вне хранилища")
	}
	if ignoreDirs[strings.Split(filepath.ToSlash(inside), "/")[0]] { // код и служебное — не карточки
		return "", errors.New("эта папка не для правки")
	}
	return full, nil
}

// ─────────────────────────── сессии ───────────────────────────
//
// Сессия — не запись в памяти, а подписанный билет: «до-когда.подпись».
// Ключ подписи выводится из хэша пароля, и из этого следует всё остальное:
//   • сервер можно перезапускать — телефон остаётся в хранилище;
//   • смена пароля рвёт все сессии сама, вести их список не нужно;
//   • хранить на сервере нечего, чистить нечего.
// Цена честная и единственная: отозвать одну сессию досрочно нечем —
// только сменой пароля (deploy/passwd.sh).

// sessionKey — ключ подписи. Хэш пароля сюда входит целиком: меняется
// пароль — меняется ключ — все прежние билеты становятся мусором.
func sessionKey() []byte {
	sum := sha256.Sum256([]byte("pillar-vault/session/v1\x00" + passHash()))
	return sum[:]
}

func signSession(exp int64) string {
	payload := strconv.FormatInt(exp, 10)
	mac := hmac.New(sha256.New, sessionKey())
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// newSession — билет на sessionTTL вперёд.
func newSession() (token string, expires time.Time) {
	expires = time.Now().Add(sessionTTL)
	return signSession(expires.Unix()), expires
}

// validSession проверяет срок и подпись. Ни то ни другое не подделать,
// не зная хэша пароля, — а он не покидает сервер.
func validSession(token string) bool {
	if passHash() == "" { // замок выключен — билетов не бывает
		return false
	}
	dot := strings.IndexByte(token, '.')
	if dot <= 0 {
		return false
	}
	exp, err := strconv.ParseInt(token[:dot], 10, 64)
	if err != nil || time.Now().Unix() >= exp {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(signSession(exp))) == 1
}

// ─────────────────── ограничение попыток входа ───────────────────

type attemptStore struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newAttemptStore() *attemptStore {
	return &attemptStore{hits: make(map[string][]time.Time)}
}

func (a *attemptStore) allow(ip string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	fresh := prune(a.hits[ip], time.Now().Add(-attemptWindow))
	// Пустую запись не оставляем: иначе сканер, перебирающий адреса,
	// оставлял бы в карте по строке на каждой — и она росла бы вечно.
	if len(fresh) == 0 {
		delete(a.hits, ip)
	} else {
		a.hits[ip] = fresh
	}
	return len(fresh) < maxAttempts
}

func (a *attemptStore) fail(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.hits[ip] = append(a.hits[ip], time.Now())
	// Адреса, которые перестали стучаться, сами из карты не уходят:
	// раз в maxTrackedIPs проходим по ней целиком и выметаем протухшее.
	if len(a.hits) > maxTrackedIPs {
		cutoff := time.Now().Add(-attemptWindow)
		for k, v := range a.hits {
			if fresh := prune(v, cutoff); len(fresh) == 0 {
				delete(a.hits, k)
			} else {
				a.hits[k] = fresh
			}
		}
	}
}

// prune оставляет только попытки свежее cutoff, переиспользуя тот же массив.
func prune(hits []time.Time, cutoff time.Time) []time.Time {
	fresh := hits[:0]
	for _, t := range hits {
		if t.After(cutoff) {
			fresh = append(fresh, t)
		}
	}
	return fresh
}

func (a *attemptStore) reset(ip string) {
	a.mu.Lock()
	delete(a.hits, ip)
	a.mu.Unlock()
}

// ─────────────────────────── пароль ───────────────────────────

// Формат: pbkdf2-sha256$<итерации>$<соль b64>$<хэш b64>
func hashPassword(pass string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum := pbkdf2SHA256([]byte(pass), salt, pbkdf2Iters, 32)
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iters,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(sum)), nil
}

func verifyPassword(pass, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false, errors.New("непонятный формат хэша")
	}
	iters, err := strconv.Atoi(parts[1])
	if err != nil || iters < 1 {
		return false, errors.New("непонятное число итераций")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false, errors.New("непонятная соль")
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false, errors.New("непонятный хэш")
	}
	got := pbkdf2SHA256([]byte(pass), salt, iters, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// pbkdf2SHA256 — PBKDF2-HMAC-SHA256 (RFC 2898) на стандартной библиотеке.
func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	hLen := sha256.Size
	blocks := (keyLen + hLen - 1) / hLen
	out := make([]byte, 0, blocks*hLen)
	buf := make([]byte, 4)
	for block := 1; block <= blocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		buf[0] = byte(block >> 24)
		buf[1] = byte(block >> 16)
		buf[2] = byte(block >> 8)
		buf[3] = byte(block)
		mac.Write(buf)
		u := mac.Sum(nil)
		t := make([]byte, len(u))
		copy(t, u)
		for i := 1; i < iter; i++ {
			mac.Reset()
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

func printHash() error {
	fmt.Fprint(os.Stderr, "Пароль (введите и нажмите Enter): ")
	line, err := bufio.NewReader(io.LimitReader(os.Stdin, 1024)).ReadString('\n')
	if err != nil && line == "" {
		return err
	}
	pass := strings.TrimRight(line, "\r\n")
	if utf8.RuneCountInString(pass) < minPassLen {
		return errors.New("пароль короче 8 символов — на сервере это не пароль")
	}
	h, err := hashPassword(pass)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "\nСохрани строку хэша в server/.passhash (файл вне репозитория) и перезапусти службу.")
	fmt.Fprintln(os.Stderr, "На сервере всё это делает одна команда: bash deploy/passwd.sh")
	fmt.Fprintln(os.Stderr)
	fmt.Println(h)
	return nil
}

// checkPassword — офлайн-проверка: тот ли пароль, что настроен на сервере.
// Нужна, когда «ключ не подошёл», а почему — непонятно.
func checkPassword() error {
	if passHash() == "" {
		return errors.New("пароль не настроен: ни PILLAR_PASS_HASH, ни server/.passhash")
	}
	fmt.Fprint(os.Stderr, "Пароль для проверки (Enter): ")
	line, err := bufio.NewReader(io.LimitReader(os.Stdin, 1024)).ReadString('\n')
	if err != nil && line == "" {
		return err
	}
	ok, err := verifyPassword(strings.TrimRight(line, "\r\n"), passHash())
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("пароль НЕ подходит к настроенному хэшу")
	}
	fmt.Fprintln(os.Stderr, "пароль подходит — сервер пустит")
	return nil
}

// passHashFromFile — локальный путь без возни с окружением: server/.passhash
func passHashFromFile() string {
	data, err := os.ReadFile(filepath.Join(root, "server", ".passhash"))
	if err != nil {
		return ""
	}
	// PowerShell пишет `>` в UTF-16 с BOM — вычищаем, иначе хэш нечитаем
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	data = bytes.TrimPrefix(data, []byte{0xFF, 0xFE})
	data = bytes.TrimPrefix(data, []byte{0xFE, 0xFF})
	data = bytes.ReplaceAll(data, []byte{0}, nil)
	line := strings.TrimSpace(string(data))
	// принимаем и голый хэш, и строку вида PILLAR_PASS_HASH='...'
	if i := strings.Index(line, "="); i != -1 && strings.HasPrefix(line, "PILLAR_PASS_HASH") {
		line = strings.Trim(strings.TrimSpace(line[i+1:]), "'\"")
	}
	return line
}

// ─────────────────────────── прочее ───────────────────────────

// watchVault следит за карточками и шаблонами и пересобирает модель, кто бы
// их ни правил — веб-редактор, внешняя правка файлов или git pull.
// Опрос вместо подписки на события: зависимостей не тянет, а хранилище невелико.
func watchVault() {
	const period = 3 * time.Second
	last := vaultStamp()
	for range time.Tick(period) {
		reloadPassHash() // passwd.sh кладёт новый хэш — перезапуск не нужен
		// шаблоны перечитываем отдельно: они не карточки, пересобирать
		// из-за них модель незачем
		if st := tmplStamp(); st != tmplStampVal.Load() {
			if err := loadTemplates(); err != nil {
				log.Printf("шаблоны не перечитаны: %v", err)
				tmplStampVal.Store(st) // не долбимся в битый файл каждые три секунды
			} else {
				log.Print("шаблоны изменились — перечитаны")
			}
		}
		now := vaultStamp()
		if now == last {
			continue
		}
		// подождать, пока правки улягутся (git pull трогает много файлов разом)
		for {
			time.Sleep(period)
			next := vaultStamp()
			if next == now {
				break
			}
			now = next
		}
		last = now
		if err := rebuild(); err != nil {
			log.Printf("пересборка после правки: %v", err)
			continue
		}
		log.Print("карточки изменились — модель пересобрана")
	}
}

// vaultStamp — отпечаток состояния карточек: число файлов и самое свежее время.
func vaultStamp() string {
	var count int
	var newest int64
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if ignoreDirs[d.Name()] { // тот же список, что у индексатора и линтера
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		count++
		if m := info.ModTime().UnixNano(); m > newest {
			newest = m
		}
		return nil
	})
	return strconv.Itoa(count) + ":" + strconv.FormatInt(newest, 10)
}

// ─────────────────────────── модель ───────────────────────────

// Хранилище живёт в памяти процесса: страницы собираются из него, на диск
// ничего не пишется. Файл, который надо не забыть пересобрать, рано или
// поздно расходится с карточками и врёт.
var (
	vault     atomic.Pointer[Vault]
	buildLock sync.Mutex
)

// currentVault — текущая модель. Пока первая сборка не прошла, отдаём пустую:
// страница должна нарисоваться и сказать об этом, а не упасть.
func currentVault() *Vault {
	if v := vault.Load(); v != nil {
		return v
	}
	return &Vault{Docs: map[string]*Doc{}, Tags: map[string]int{},
		Tree: &treeNode{Name: ".", Path: ".", Dirs: map[string]*treeNode{}}}
}

// rebuild перечитывает карточки — своими силами, без внешних процессов.
func rebuild() error {
	buildLock.Lock()
	defer buildLock.Unlock()
	v, err := buildIndex(root)
	if err != nil {
		return fmt.Errorf("сборка модели: %w", err)
	}
	vault.Store(v)
	return nil
}

// isFlagSet — задавали ли флаг явно (иначе можно взять значение из окружения)
func isFlagSet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// clientIP — адрес для счётчика попыток входа. Заголовку верим, только если
// запрос пришёл от своего nginx на loopback: X-Real-IP он ставит сам из
// $remote_addr. Первый элемент X-Forwarded-For пишет клиент — по нему счётчик
// обходился подменой заголовка на каждой попытке.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
			return real
		}
	}
	return host
}

// logging пишет в журнал записи — POST: кто и когда правил хранилище с веба.
// Чтение страниц не пишется: шум без пользы.
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if r.Method == http.MethodPost {
			log.Printf("%s %s %s %s", clientIP(r), r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		}
	})
}
