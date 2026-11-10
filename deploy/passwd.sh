#!/usr/bin/env bash
# ── Pillar Vault — смена пароля с терминала ───────────────────────────────
# Пароль живёт хэшем в server/.passhash (0600, вне git). Скрипт считает хэш,
# кладёт файл и ждёт, пока служба подхватит его сама (перезапуск — только
# для старого бинаря); если проверка не прошла — возвращает прежний файл.
# С телефона то же самое делает страница «Доступ» (🔑 в шапке).
#
# Сессии на всех устройствах рвутся сами: билет подписан ключом, выведенным
# из хэша пароля (server/main.go, sessionKey). Новый хэш — прежние билеты
# мусор. Отдельно ничего чистить не нужно.
#
# Запуск:  bash deploy/passwd.sh
set -euo pipefail

REPO_DIR="${PILLAR_VAULT_DIR:-/opt/pillar-vault}"
SERVICE="pillar-vault.service"
BIN="server/pillar-vault-server"
HASH_FILE="server/.passhash"
GO="${GO:-/usr/local/go/bin/go}"
MIN_LEN=8   # столько же требует сервер (printHash в server/main.go)

log() { printf '[%s] %s\n' "$(date '+%F %T')" "$*"; }
die() { printf '✗ %s\n' "$*" >&2; exit 1; }

cd "$REPO_DIR" || die "нет каталога $REPO_DIR"
[ -f SOUL.md ] || die "$REPO_DIR — не корень хранилища"

if [ ! -x "$BIN" ]; then
  command -v "$GO" >/dev/null || die "нет ни бинаря, ни Go ($GO)"
  log "бинаря нет — собираю"
  "$GO" build -trimpath -ldflags="-s -w" -o "$BIN" ./server
fi

# ── ввод ─────────────────────────────────────────────────────────────────
# Пароль читается с терминала и не попадает ни в argv, ни в историю оболочки.
[ -t 0 ] || die "нужен терминал: пароль читается вводом, а не из пайпа"
read -rsp "Новый пароль (не короче $MIN_LEN символов): " pass1; echo
read -rsp "Повторите пароль: " pass2; echo

[ "$pass1" = "$pass2" ]        || die "пароли не совпали"
[ "${#pass1}" -ge "$MIN_LEN" ] || die "пароль короче $MIN_LEN символов — на сервере это не пароль"

# ── хэш ──────────────────────────────────────────────────────────────────
# -hash читает пароль со stdin и печатает строку хэша в stdout
# (подсказки уходят в stderr, поэтому их здесь не видно).
hash=$(printf '%s\n' "$pass1" | "./$BIN" -hash 2>/dev/null) || die "не удалось посчитать хэш"
case "$hash" in
  pbkdf2-sha256\$*) : ;;
  *) die "бинарь вернул не хэш — пересобери сервер" ;;
esac

# ── файл ─────────────────────────────────────────────────────────────────
backup=""
if [ -f "$HASH_FILE" ]; then
  backup="$HASH_FILE.bak.$(date '+%Y%m%d-%H%M%S')"
  cp -a "$HASH_FILE" "$backup"
  log "прежний хэш сохранён: $backup"
fi

umask 077
printf '%s\n' "$hash" > "$HASH_FILE.tmp"   # через временный: служба не прочтёт полстроки
chmod 600 "$HASH_FILE.tmp"
mv -f "$HASH_FILE.tmp" "$HASH_FILE"
log "✓ $HASH_FILE обновлён"

restore() {
  [ -n "$backup" ] || return 0
  log "возвращаю прежний хэш"
  cp -a "$backup" "$HASH_FILE"
  sudo systemctl restart "$SERVICE" >/dev/null 2>&1 || true
}

# ── проверка ─────────────────────────────────────────────────────────────
# Сверяем новый пароль тем же кодом, что проверяет вход (-check).
if ! printf '%s\n' "$pass1" | "./$BIN" -root "$REPO_DIR" -check >/dev/null 2>&1; then
  restore
  die "новый пароль не сходится с хэшем — файл возвращён"
fi

# Служба следит за server/.passhash и подхватывает новый хэш сама, без
# перезапуска (reloadPassHash в server/access.go). Ждём, пока настоящий вход
# новым паролем выдаст сессию: так проверена вся цепочка, а не только файл.
# Пароль уходит curl-у через stdin (@-), не через argv. В журнале входов
# это видно как «вход» с 127.0.0.1.
login_ok() {
  printf '%s' "$pass1" | curl -sS -o /dev/null -D - --max-time 5 \
      --data-urlencode "password@-" http://127.0.0.1:8080/login 2>/dev/null \
    | grep -qi '^set-cookie: pv_session=[^;]'
}

picked=""
for _ in 1 2 3 4 5 6 7 8 9 10; do
  sleep 1
  if login_ok; then picked=1; break; fi
done

if [ -z "$picked" ]; then
  # старый бинарь читал хэш только при старте — тогда перезапуск
  log "служба не подхватила хэш сама — перезапускаю"
  if ! sudo systemctl restart "$SERVICE"; then
    restore
    die "служба не поднялась — хэш возвращён"
  fi
  sleep 2
  login_ok || { restore; die "новый пароль не пускает — хэш возвращён"; }
fi

rm -f "$backup"
log "✓ пароль сменён, сессии на всех устройствах закрыты"
