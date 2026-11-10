#!/usr/bin/env bash
# ── Pillar Vault — обновление на VDS ──────────────────────────────────────
# Идемпотентен: если в git ничего нового, ничего не пересобирает. Поэтому его
# не жалко дёргать часто и из нескольких мест сразу:
#   • cron пользователя deploy — каждые 10 минут: синхронизация правок с веба;
#   • pillar-vault-deploy.timer — раз в сутки, страховка;
#   • .github/workflows/deploy.yml — сразу после push, ради скорости.
# Настройка обоих — в DEPLOY.md.
#
# Docker здесь больше нет: собирается бинарь, службу держит systemd.
# Сборка образа на одноядерном VDS занимала минуты, а супервизор в системе
# и так уже был.
set -euo pipefail

# Всё тело — в фигурных скобках: bash разбирает блок целиком до запуска.
# Иначе git pull, переписав этот же файл посреди прогона, подсовывал бы
# bash новый текст по старому смещению.
{

REPO_DIR="${PILLAR_VAULT_DIR:-/opt/pillar-vault}"
SERVICE="pillar-vault.service"
BIN="server/pillar-vault-server"
GO="${GO:-/usr/local/go/bin/go}"

log() { printf '[%s] %s\n' "$(date '+%F %T')" "$*"; }
die() { log "✗ $*" >&2; exit 1; }

cd "$REPO_DIR" || die "нет каталога $REPO_DIR"
[ -d .git ] || die "$REPO_DIR — не git-репозиторий"

# Таймер и пинок из Actions могут прийти одновременно. Второй ждёт первого,
# а не собирает бинарь поверх идущей сборки.
exec 9>"/tmp/pillar-vault-deploy.lock"
flock -w 600 9 || die "предыдущий деплой идёт больше 10 минут — пропускаю такт"

# ── правки, сделанные на самом сервере ──────────────────────────────────
# Веб коммитит свои правки сам (server/sync.go). Всё, что осталось
# незакоммиченным, подбираем одним коммитом — иначе pull споткнулся бы
# о грязную рабочую копию и деплой встал.
# Только markdown: бинарь, хэш пароля и прочее лежат вне git или в .gitignore.
git add -A -- '*.md'
if ! git diff --cached --quiet; then
  git -c user.name="Pillar Vault" -c user.email=vault@vault.example.com \
      commit --quiet -m "сервер: правки карточек на месте" \
    || die "не удалось закоммитить правки на сервере"
  log "правки на сервере закоммичены"
fi

before=$(git rev-parse HEAD)
# rebase, а не ff-only: у сервера бывают свои коммиты (правки с веба), и они
# встают поверх пришедшего с GitHub. Конфликт — откатываем rebase и стоим:
# лучше вчерашняя версия, чем карточка с маркерами конфликта.
if ! git pull --rebase --quiet; then
  git rebase --abort 2>/dev/null || true
  die "git pull --rebase не прошёл — конфликт правок с веба и из git, разреши руками"
fi
after=$(git rev-parse HEAD)

# Свои коммиты — на GitHub: там их увидит и локальная копия.
if [ -n "$(git rev-list '@{u}..HEAD' 2>/dev/null)" ]; then
  if git push --quiet; then
    log "правки с сервера отправлены в git"
  else
    log "⚠ git push не прошёл — отправлю на следующем такте"
  fi
fi

# ── свежий APK для кнопки «Обновить приложение» ─────────────────────────
# CI кладёт сборку в ветку android-apk (один перезаписываемый коммит).
# Достаём в server/app/ — вне git, отдаёт server/appupdate.go по сессии.
# Сбой здесь деплой не останавливает: приложение обновится на следующем такте.
if git fetch --quiet origin '+refs/heads/android-apk:refs/remotes/origin/android-apk' 2>/dev/null; then
  apk_blob=$(git rev-parse --verify --quiet 'origin/android-apk:pillar-vault.apk' || true)
  if [ -n "$apk_blob" ] && [ "$apk_blob" != "$(cat server/app/.blob 2>/dev/null || true)" ]; then
    mkdir -p server/app
    # сперва APK, потом версия: сервер не должен обещать сборку, которой ещё нет
    if git cat-file blob "$apk_blob" > server/app/pillar-vault.apk.new \
       && git show 'origin/android-apk:version.json' > server/app/version.json.new; then
      mv server/app/pillar-vault.apk.new server/app/pillar-vault.apk
      mv server/app/version.json.new server/app/version.json
      printf '%s\n' "$apk_blob" > server/app/.blob
      log "APK приложения обновлён: $(cat server/app/version.json)"
    else
      rm -f server/app/*.new
      log "⚠ APK из ветки android-apk не извлёкся — попробую на следующем такте"
    fi
  fi
fi

# ── React-приложение (web/) ─────────────────────────────────────────────
# CI собирает его в ветку web-dist (один перезаписываемый коммит), здесь
# только распаковка в viewer/x/ — Node на сервере не нужен. Папка
# подменяется целиком: открытая вкладка не увидит сборку наполовину.
# Сбой не останавливает деплой: приложение обновится на следующем такте.
if git fetch --quiet origin '+refs/heads/web-dist:refs/remotes/origin/web-dist' 2>/dev/null; then
  web_tree=$(git rev-parse --verify --quiet 'origin/web-dist^{tree}' || true)
  if [ -n "$web_tree" ] && [ "$web_tree" != "$(cat viewer/x/.tree 2>/dev/null || true)" ]; then
    rm -rf viewer/x.new && mkdir -p viewer/x.new
    if git archive origin/web-dist | tar -x -C viewer/x.new; then
      printf '%s\n' "$web_tree" > viewer/x.new/.tree
      rm -rf viewer/x.old
      [ -d viewer/x ] && mv viewer/x viewer/x.old
      mv viewer/x.new viewer/x && rm -rf viewer/x.old
      log "приложение обновлено: $(cat viewer/x/build.json 2>/dev/null || echo '?')"
    else
      rm -rf viewer/x.new
      log "⚠ ветка web-dist не распаковалась — попробую на следующем такте"
    fi
  fi
fi

changed=""
if [ "$before" != "$after" ]; then
  changed=$(git diff --name-only "$before" "$after")
  log "новое: ${before:0:7} → ${after:0:7} ($(printf '%s\n' "$changed" | wc -l) файлов)"
else
  log "изменений нет"
fi

# Пересобирать нужно, только если менялся код сервера. Карточки сервер
# подхватывает с диска сам и пересобирает индекс без перезапуска.
#
# Сравниваем с коммитом последней УДАЧНОЙ сборки (server/.built), а не с тем,
# что было до pull. Иначе сборка, упавшая на тестах, больше не повторялась:
# следующий такт видел «изменений нет», и сервер навсегда оставался на старом
# бинаре. Упавший коммит помнится в server/.build-failed — его не пересобираем
# каждые 10 минут, пробуем снова, когда придёт новый.
BUILT="server/.built"
FAILED="server/.build-failed"
head=$(git rev-parse HEAD)
need_build=no
fresh=no # бинарь соответствует HEAD — тогда HEAD и запоминается как собранный
built=$(cat "$BUILT" 2>/dev/null || true)
if [ ! -x "$BIN" ]; then
  need_build=yes; log "бинаря нет — собираю"
elif [ -z "$built" ] || ! git cat-file -e "$built^{commit}" 2>/dev/null; then
  need_build=yes; log "неизвестно, из какого коммита бинарь — пересобираю"
else
  # список целиком в переменную: grep -q на конвейере под pipefail мог бы
  # оборвать git diff сигналом и молча прочесть «изменений нет»
  since=$(git diff --name-only "$built" "$head")
  if printf '%s\n' "$since" | grep -qE '^(server/.*\.go|go\.(mod|sum))$'; then
    need_build=yes; log "код сервера менялся с ${built:0:7} — пересобираю"
  else
    fresh=yes
  fi
fi
if [ "$need_build" = yes ] && [ "$(cat "$FAILED" 2>/dev/null || true)" = "$head" ]; then
  need_build=no
  log "⚠ сборка ${head:0:7} уже падала — жду нового коммита, работает прежний бинарь"
fi
# Проверки и сборка до первой удачи помечают коммит упавшим: die посреди
# них оставляет метку, удачный финал её снимает.
fail_mark() { printf '%s\n' "$head" > "$FAILED"; }

if [ "$need_build" = yes ]; then
  command -v "$GO" >/dev/null || die "нет Go ($GO) — поставь toolchain, см. DEPLOY.md"

  # Проверки идут до подмены бинаря. Таймер тянет git и не видит CI —
  # значит, ворота должны стоять здесь же, на самой машине.
  log "проверки: vet, тесты, линтер хранилища"
  "$GO" vet ./... || { fail_mark; die "go vet не прошёл — старый бинарь остаётся работать"; }
  "$GO" test ./... -count=1 || { fail_mark; die "тесты не прошли — старый бинарь остаётся работать"; }
  "$GO" run ./server -root "$REPO_DIR" -lint || log "⚠ линтер ругается на хранилище (деплой не останавливаю)"

  log "сборка"
  "$GO" build -trimpath -ldflags="-s -w" -o "$BIN.new" ./server || { fail_mark; die "сборка не прошла"; }
  [ -x "$BIN" ] && cp -a "$BIN" "$BIN.bak"
  mv "$BIN.new" "$BIN"
  fresh=yes

  log "перезапуск службы"
  sudo systemctl restart "$SERVICE" || die "systemctl restart не прошёл"
else
  # Служба могла упасть между тактами: start поднимет её, а если всё живо —
  # это пустая операция.
  sudo systemctl start "$SERVICE" || die "systemctl start не прошёл"
fi

# ── проверка живости ─────────────────────────────────────────────────────
# Без сессии сервер отдаёт форму входа — 200 на «/» и означает, что контур
# поднялся. Даём ему секунду на старт.
ok=no
for _ in 1 2 3 4 5; do
  sleep 1
  if curl -fsS -o /dev/null --max-time 3 http://127.0.0.1:8080/; then ok=yes; break; fi
done

if [ "$ok" = yes ]; then
  log "✓ готово, хранилище отвечает"
  rm -f "$BIN.bak"
  if [ "$fresh" = yes ]; then rm -f "$FAILED"; fi
  # собран сейчас или код сервера с прошлой сборки не менялся
  if [ "$fresh" = yes ]; then printf '%s\n' "$head" > "$BUILT"; fi
  exit 0
fi

# Не поднялось. Если есть прежний бинарь — возвращаем его: лучше вчерашняя
# версия на ходу, чем свежая в кювете.
if [ "$need_build" = yes ]; then fail_mark; fi
if [ -f "$BIN.bak" ]; then
  log "⚠ сервер не отвечает — откатываю бинарь"
  mv "$BIN.bak" "$BIN"
  sudo systemctl restart "$SERVICE" || true
  sleep 2
  if curl -fsS -o /dev/null --max-time 3 http://127.0.0.1:8080/; then
    die "новая версия не поднялась, вернул прежнюю — смотри: journalctl -u $SERVICE -n 50"
  fi
fi
die "сервер не отвечает — смотри: journalctl -u $SERVICE -n 50"
}
