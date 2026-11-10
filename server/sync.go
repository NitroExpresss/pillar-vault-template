// Автокоммит правок с веба.
//
// Дела, дневник и столпы с телефона пишутся в markdown на сервере. Без
// коммита они жили только в рабочей копии: не доезжали до git, не попадали
// к агенту и останавливали деплой, если тот же файл приходил с push.
//
// Сервер коммитит сам, но не отправляет: сеть наружу ему закрыта песочницей
// systemd. Отправляет deploy/deploy.sh — pull --rebase и push по таймеру.
//
// Включается файлом server/.autocommit (вне git) или PILLAR_AUTOCOMMIT=1:
// локальный запуск на машине разработчика коммитить в его репозиторий не должен.
package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var (
	autoCommit bool
	gitMu      sync.Mutex // два сохранения подряд не должны делить .git/index.lock
)

func initAutoCommit() {
	_, err := os.Stat(filepath.Join(root, "server", ".autocommit"))
	autoCommit = err == nil || os.Getenv("PILLAR_AUTOCOMMIT") == "1"
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		autoCommit = false
	}
	if autoCommit {
		log.Print("автокоммит включён: правки с веба уходят в git")
	}
}

// commitFiles коммитит перечисленные файлы одним коммитом. Ошибка не роняет
// запрос: правка уже на диске, а незакоммиченное подберёт deploy.sh.
func commitFiles(what string, rels ...string) {
	if !autoCommit || len(rels) == 0 {
		return
	}
	gitMu.Lock()
	defer gitMu.Unlock()
	msg := "веб: " + cutRunes(strings.Join(strings.Fields(what), " "), 90)
	if out, err := git(append([]string{"add", "--"}, rels...)...); err != nil {
		log.Printf("автокоммит: git add: %v %s", err, out)
		return
	}
	args := append([]string{"-c", "user.name=Pillar Vault", "-c", "user.email=vault@vault.example.com",
		"commit", "--quiet", "-m", msg, "--"}, rels...)
	if out, err := git(args...); err != nil && !strings.Contains(out, "nothing to commit") {
		log.Printf("автокоммит: git commit: %v %s", err, out)
	}
}

func git(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	// HOME в песочнице закрыт (ProtectHome) — пусть git не ищет там конфиг
	cmd.Env = append(os.Environ(), "HOME="+os.TempDir(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
