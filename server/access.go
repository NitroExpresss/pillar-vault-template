package main

// Доступ: журнал входов и смена пароля без перезапуска службы.
//
// Хэш пароля держится в памяти атомарно: его меняют на лету форма «Доступ»
// и файл server/.passhash, который кладёт deploy/passwd.sh. Ключ подписи
// сессий выведен из хэша (sessionKey), поэтому новый пароль сам закрывает
// все прежние сессии — чистить ничего не нужно.

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

const (
	loginLogFile = "server/.logins.log" // вне git, как и .passhash
	loginLogMax  = 512 << 10            // дорос — обрезается до последних loginLogKeep строк
	loginLogKeep = 2000
	loginShown   = 50
	minPassLen   = 8 // столько же требуют -hash и deploy/passwd.sh

	loginOK          = "вход"
	loginDenied      = "ключ не подошёл"
	loginBlocked     = "отбит: много попыток"
	loginPassChanged = "пароль сменён"
	loginPassFile    = "пароль сменён в файле"
	loginPassDenied  = "смена пароля: старый ключ не подошёл"
)

var (
	passHashVal atomic.Value // string; пусто — замок выключен
	hashFromEnv bool         // хэш из PILLAR_PASS_HASH: файл не следим, с веба не меняем
	// отпечаток server/.passhash; трогают только main до старта и watchVault
	hashFileStamp string
	loginLogMu    sync.Mutex
)

func passHash() string {
	s, _ := passHashVal.Load().(string)
	return s
}

func setPassHash(h string) { passHashVal.Store(h) }

func passHashPath() string { return filepath.Join(root, "server", ".passhash") }

// passHashStamp — размер и время файла хэша; пусто, если файла нет.
func passHashStamp() string {
	info, err := os.Stat(passHashPath())
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano())
}

// reloadPassHash подхватывает новый server/.passhash. Пустой или битый файл
// замок не снимает и пароль не портит: остаётся прежний хэш — иначе опечатка
// в файле выключала бы вход или открывала бы его чем попало.
func reloadPassHash() {
	if hashFromEnv {
		return
	}
	st := passHashStamp()
	if st == hashFileStamp {
		return
	}
	hashFileStamp = st
	h := passHashFromFile()
	if h == "" || h == passHash() {
		if h == "" && passHash() != "" {
			log.Print("server/.passhash пуст или пропал — оставлен прежний пароль")
		}
		return
	}
	if _, err := verifyPassword("", h); err != nil {
		log.Printf("новый server/.passhash не читается (%v) — оставлен прежний пароль", err)
		return
	}
	setPassHash(h)
	log.Print("пароль сменён в server/.passhash — подхвачен без перезапуска, прежние сессии закрыты")
	appendLoginLog(loginEntry{When: time.Now(), IP: "сервер", Result: loginPassFile})
}

// writePassHash кладёт хэш в файл атомарно: временный файл и переименование,
// чтобы watchVault никогда не прочёл половину строки.
func writePassHash(h string) error {
	tmp := passHashPath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(h+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, passHashPath()); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// ─────────────────────────── журнал входов ───────────────────────────

type loginEntry struct {
	When   time.Time
	IP     string
	Result string
	Agent  string
}

func (e loginEntry) At() string { return e.When.Format("02.01.2006 15:04:05") }
func (e loginEntry) OK() bool {
	return e.Result == loginOK || strings.HasPrefix(e.Result, loginPassChanged)
}

// Device — коротко, с чего входили: полную строку браузера показывает подсказка.
func (e loginEntry) Device() string {
	ua := e.Agent
	if ua == "" {
		return "—"
	}
	pick := func(pairs ...string) string {
		for i := 0; i+1 < len(pairs); i += 2 {
			if strings.Contains(ua, pairs[i]) {
				return pairs[i+1]
			}
		}
		return ""
	}
	osName := pick("PillarVaultApp", "приложение", "Android", "Android", "iPhone", "iPhone", "iPad", "iPad",
		"Windows", "Windows", "Mac OS", "Mac", "Linux", "Linux")
	app := pick("curl/", "curl", "YaBrowser", "Яндекс", "Edg/", "Edge", "Firefox/", "Firefox",
		"Chrome/", "Chrome", "Safari/", "Safari")
	switch {
	case osName == "приложение":
		return "приложение Android"
	case osName != "" && app != "":
		return osName + " · " + app
	case osName+app != "":
		return osName + app
	}
	return "другое"
}

func logLogin(r *http.Request, result string) {
	ip := clientIP(r)
	log.Printf("доступ: %s — %s", ip, result)
	appendLoginLog(loginEntry{When: time.Now(), IP: ip, Result: result, Agent: r.UserAgent()})
}

// clean — одна строка без табов: журнал разбирается по ним.
func clean(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, s)
	if rs := []rune(s); len(rs) > max {
		s = string(rs[:max])
	}
	return s
}

func appendLoginLog(e loginEntry) {
	loginLogMu.Lock()
	defer loginLogMu.Unlock()
	full := filepath.Join(root, filepath.FromSlash(loginLogFile))
	line := strings.Join([]string{e.When.Format(time.RFC3339), clean(e.IP, 64),
		clean(e.Result, 80), clean(e.Agent, 240)}, "\t") + "\n"
	f, err := os.OpenFile(full, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		log.Printf("журнал входов: %v", err)
		return
	}
	_, err = f.WriteString(line)
	info, _ := f.Stat()
	f.Close()
	if err != nil {
		log.Printf("журнал входов: %v", err)
		return
	}
	if info != nil && info.Size() > loginLogMax {
		trimLoginLog(full)
	}
}

// trimLoginLog оставляет последние loginLogKeep строк: сканер, перебирающий
// пароли, не должен раздуть файл до конца диска.
func trimLoginLog(full string) {
	raw, err := os.ReadFile(full)
	if err != nil {
		return
	}
	lines := bytes.Split(bytes.TrimRight(raw, "\n"), []byte("\n"))
	if len(lines) <= loginLogKeep {
		return
	}
	kept := append(bytes.Join(lines[len(lines)-loginLogKeep:], []byte("\n")), '\n')
	tmp := full + ".tmp"
	if os.WriteFile(tmp, kept, 0o600) == nil {
		if os.Rename(tmp, full) != nil {
			os.Remove(tmp)
		}
	}
}

// readLogins — последние n записей, свежие сверху.
func readLogins(n int) []loginEntry {
	loginLogMu.Lock()
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(loginLogFile)))
	loginLogMu.Unlock()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	var out []loginEntry
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		f := strings.SplitN(lines[i], "\t", 4)
		if len(f) < 3 {
			continue
		}
		when, err := time.Parse(time.RFC3339, f[0])
		if err != nil {
			continue
		}
		e := loginEntry{When: when.Local(), IP: f[1], Result: f[2]}
		if len(f) == 4 {
			e.Agent = f[3]
		}
		out = append(out, e)
	}
	return out
}

// ─────────────────────────── страница «Доступ» ───────────────────────────

// handleAccess: GET — журнал входов и форма; POST — смена пароля. Старый
// пароль спрашивается и под сессией: забытый незакрытым телефон не должен
// давать сменить ключ и выставить хозяина за дверь.
func handleAccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handleRoot(w, r) // экран «Доступ» рисует приложение, журнал — /api/access
		return
	}
	if !requirePost(w, r) {
		return
	}
	back := func(msg string) { http.Redirect(w, r, urlWithMsg("/access", msg), http.StatusSeeOther) }
	if hashFromEnv {
		back("пароль задан в окружении PILLAR_PASS_HASH — меняется там")
		return
	}
	ip := clientIP(r)
	if !attempts.allow(ip) {
		logLogin(r, loginBlocked)
		back("слишком много попыток, подождите")
		return
	}
	ok, err := verifyPassword(r.FormValue("old"), passHash())
	if err != nil {
		back("сервер не смог проверить ключ")
		return
	}
	if !ok {
		attempts.fail(ip)
		logLogin(r, loginPassDenied)
		time.Sleep(400 * time.Millisecond)
		back("старый ключ не подошёл")
		return
	}
	next := r.FormValue("new")
	switch {
	case utf8.RuneCountInString(next) < minPassLen: // буквы, а не байты: кириллица вдвое длиннее
		back(fmt.Sprintf("новый ключ короче %d символов", minPassLen))
		return
	case next != r.FormValue("repeat"):
		back("новый ключ и повтор не совпали")
		return
	}
	h, err := hashPassword(next)
	if err == nil {
		err = writePassHash(h)
	}
	if err != nil {
		log.Printf("смена пароля: %v", err)
		back("ключ не сменён: сервер не смог записать файл")
		return
	}
	attempts.reset(ip)
	setPassHash(h)
	logLogin(r, loginPassChanged)
	setSessionCookie(w, r) // это устройство остаётся внутри, остальные — за дверью
	markOK(w)
	back("ключ сменён — остальные устройства вышли")
}
