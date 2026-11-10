# 🚀 Деплой Pillar Vault на VDS

Схема: **nginx с TLS** → **служба systemd на 127.0.0.1:8080**. Пароль проверяет
сам сервер, поэтому Basic Auth поверх не нужен. Граница доступа —
[`../server/README.md`](../server/README.md).

```
push в main ─┬─ workflow deploy.yml ─ проверки ─ ssh ─┐
             │                                        ├─→ deploy.sh на VDS
             └─ (запасной путь) systemd-таймер ────────┘   git pull, сборка,
                                                           systemctl restart
```

Автодеплой держится на **workflow**: он дёргает скрипт сразу после push, но
только если прошли `go vet`, тесты и линтер хранилища. Systemd-таймер —
страховка: раз в сутки в 04:00 сервер сам проверяет git. Таймер проверок из
Actions не видит, поэтому тот же набор гоняет и `deploy.sh` перед подменой
бинаря: ворота стоят на самой машине, а не только в CI.

> Docker здесь был до 2026-09-09 и снят намеренно. Сборка образа на одном
> ядре занимала минуты, супервизор в системе и так уже был (таймер деплоя),
> а изоляцию distroless-контейнера systemd повторяет директивами
> `Protect*`/`SystemCall*` в [`pillar-vault.service`](pillar-vault.service).

---

## Разовая настройка

### 1. Репозиторий

```bash
sudo mkdir -p /opt && cd /opt
sudo git clone git@github.com:NitroExpresss/pillar-vault-template.git
sudo chown -R "$USER":"$USER" /opt/pillar-vault
cd /opt/pillar-vault
```

Репозиторий приватный → нужен deploy-ключ на сервере. Создать и добавить в
Settings → Deploy keys (read-only достаточно, автодеплой только читает):

```bash
ssh-keygen -t ed25519 -C "pillar-vault vds" -f ~/.ssh/id_pillar -N ''
cat ~/.ssh/id_pillar.pub        # → GitHub → Settings → Deploy keys → Add
```

### 2. Go

Нужен на самой машине: деплой собирает бинарь из исходников, а не тянет
готовый артефакт — иначе суточный таймер не смог бы обновиться в одиночку.

```bash
ver=$(curl -sS 'https://go.dev/VERSION?m=text' | head -1)
curl -fsSL -o /tmp/go.tgz "https://go.dev/dl/$ver.linux-amd64.tar.gz"
sha256sum /tmp/go.tgz           # сверить с https://go.dev/dl/
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf /tmp/go.tgz
/usr/local/go/bin/go version
```

### 3. Пароль от замка

```bash
cd /opt/pillar-vault
/usr/local/go/bin/go build -trimpath -ldflags="-s -w" -o server/pillar-vault-server ./server
bash deploy/passwd.sh           # спросит пароль дважды и всё сделает само
```

Хэш ложится в `server/.passhash` (0600, вне git). Без него войти нельзя вовсе —
карточки не отдаются никому, в браузер уходит пустая витрина. Это безопасное
состояние, а не поломка.

### 4. Служба

```bash
cd /opt/pillar-vault
sudo cp deploy/pillar-vault.service /etc/systemd/system/
sudo sed -i "s/^User=.*/User=$USER/;s/^Group=.*/Group=$(id -gn)/" \
     /etc/systemd/system/pillar-vault.service
sudo systemctl daemon-reload
sudo systemctl enable --now pillar-vault.service
curl -sI localhost:8080/ | head -1              # ждём 200 (форма входа)
```

### 5. nginx и TLS

```bash
sudo cp deploy/nginx-pillar-vault.conf /etc/nginx/sites-available/pillar-vault
sudo nano /etc/nginx/sites-available/pillar-vault      # server_name под себя
sudo ln -s /etc/nginx/sites-available/pillar-vault /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
sudo apt-get install -y certbot python3-certbot-nginx
sudo certbot --nginx -d vault.example.com -d www.vault.example.com
```

certbot сам допишет `listen 443 ssl`, сертификат и редирект с 80 → 443. HTTPS
обязателен: по голому HTTP пароль и кука сессии уходят открытым текстом.

### 6. Право на перезапуск

Деплою нужно перезапускать службу — и больше ничего:

```bash
cd /opt/pillar-vault
sudo sed -i "s/^deploy /$USER /" deploy/pillar-vault.sudoers   # если пользователь другой
sudo install -m 0440 -o root -g root deploy/pillar-vault.sudoers /etc/sudoers.d/pillar-vault
sudo visudo -c                  # обязательно: опечатка тут закрывает sudo всем
```

### 7. Таймер — основа автодеплоя

```bash
cd /opt/pillar-vault
sudo cp deploy/pillar-vault-deploy.{service,timer} /etc/systemd/system/
sudo sed -i "s/^User=.*/User=$USER/" /etc/systemd/system/pillar-vault-deploy.service
sudo systemctl daemon-reload
sudo systemctl enable --now pillar-vault-deploy.timer
```

Проверить, что живой:

```bash
systemctl list-timers pillar-vault-deploy      # когда следующий такт
sudo systemctl start pillar-vault-deploy       # прогнать прямо сейчас
journalctl -u pillar-vault-deploy -n 30        # что он сделал
```

На этом автодеплой уже работает. Шаг 8 — только про скорость.

### 8. Мгновенный пинок из GitHub Actions

Ключ **отдельный от deploy-ключа из шага 1**: тот пускает сервер в GitHub, этот
пускает GitHub на сервер. На VDS:

```bash
ssh-keygen -t ed25519 -C "github actions → vds" -f ~/.ssh/id_gha -N ''
cat ~/.ssh/id_gha.pub >> ~/.ssh/authorized_keys
cat ~/.ssh/id_gha          # приватный — целиком в секрет VDS_SSH_KEY
ssh-keyscan -p 22 vault.example.com 2>/dev/null   # → в секрет VDS_KNOWN_HOSTS
```

Затем Settings → Secrets and variables → Actions → New repository secret:

| Секрет | Что кладём |
|---|---|
| `VDS_HOST` | адрес сервера |
| `VDS_USER` | владелец `/opt/pillar-vault` |
| `VDS_SSH_KEY` | приватный ключ целиком, со строками BEGIN/END |
| `VDS_KNOWN_HOSTS` | вывод `ssh-keyscan` |
| `VDS_PORT` | необязательно, по умолчанию 22 |

Пока `VDS_HOST` не задан, workflow тихо пропускает себя и не красит push.
Проверить — Actions → deploy → Run workflow.

Понять, настроен ли быстрый путь вообще, можно и с самого VDS — не заглядывая
в секреты репозитория:

```bash
grep -c 'github actions' ~/.ssh/authorized_keys   # 0 → шаг 8 не выполнен
```

Ноль означает, что автодеплой держится на одном таймере: изменения приезжают
в 04:00, а не через минуту после push. Это рабочее состояние, а не поломка, —
но знать о нём надо, иначе ждёшь обновления, которого сегодня не будет.

> Ключ пускает на сервер кого угодно с доступом к секретам репозитория. Хочешь
> сузить — в `~/.ssh/authorized_keys` перед ключом допиши
> `command="bash /opt/pillar-vault/deploy/deploy.sh",no-pty,no-port-forwarding`:
> тогда этим ключом нельзя ничего, кроме деплоя.

---

## Как это работает дальше

Правишь карточки на рабочей машине → `git push` → в течение минуты (или до
суток, если Actions пропущены) сервер подтягивает изменения.

`deploy.sh` идемпотентен и различает три случая:

| Что изменилось в push | Что делает деплой |
|---|---|
| только карточки `.md` | `git pull`, и всё — сервер сам заметит файлы на диске и пересоберёт модель, перезапуск не нужен |
| шаблоны `viewer/templates/*.html` | `git pull`, и всё — сервер перечитает их на лету |
| `server/*.go`, `go.mod` | `git pull` → `go vet` → тесты → сборка → `systemctl restart` |
| ничего (такт вхолостую) | молча выходит, попутно подняв службу, если та лежит |

Тесты не прошли — бинарь не подменяется, служба продолжает работать на прежнем.
Новый бинарь не поднялся — деплой возвращает предыдущий и перезапускает службу
обратно. Молчаливой поломки прода не будет.

Таймер и пинок из Actions не мешают друг другу: скрипт держит `flock`, второй
дожидается первого.

### Правки с веба и с телефона

Дела, дневник, столпы и правка карточек с сайта пишутся в markdown прямо на
сервере. Чтобы они не жили только в рабочей копии:

1. **Сервер коммитит каждую правку сам** (`server/sync.go`) — по флажку
   `server/.autocommit` (вне git). Отправить не может: сеть наружу ему закрыта
   песочницей systemd.
2. **`deploy.sh` синхронизирует:** подбирает незакоммиченные `.md` (их правит
   и агент на сервере), делает `git pull --rebase` — свои коммиты встают поверх
   пришедших — и `git push`. Конфликт правок — rebase откатывается, деплой
   встаёт с понятной ошибкой, карточки без маркеров конфликта.
3. **Cron пользователя `deploy` раз в 10 минут** — системный таймер раз в
   сутки для синхронизации редок, а менять его без root нельзя:

```bash
touch /opt/pillar-vault/server/.autocommit && sudo systemctl restart pillar-vault
( crontab -l 2>/dev/null | grep -v pillar-vault-sync
  echo '*/10 * * * * systemd-cat -t pillar-vault-sync /bin/bash /opt/pillar-vault/deploy/deploy.sh' ) | crontab -
journalctl -t pillar-vault-sync -n 20        # что делала синхронизация
```

Скрипт запускается **от владельца** `/opt/pillar-vault` (`deploy`), не от root:
`sudo -u deploy -H bash /opt/pillar-vault/deploy/deploy.sh`. От root он упрётся
в чужой файл блокировки в `/tmp`, а `git pull` под root оставил бы в `.git`
файлы, которые cron потом не сможет перезаписать.

**Когда пересобирается бинарь.** Сравнивается коммит последней удачной сборки
(`server/.built`, вне git) с текущим: менялся код сервера — сборка. Если сборка
или тесты упали, коммит помечается в `server/.build-failed`: работает прежний
бинарь, и этот коммит не пересобирается каждые 10 минут — следующая попытка
с приходом нового. Первый запуск после появления `.built` один раз пересоберёт
бинарь: из какого коммита собран старый, неизвестно.

Ключ сервера для GitHub нужен с правом записи (deploy key, read-write).

**Смена пароля** автодеплоем не покрыта и не должна быть: `bash deploy/passwd.sh`
на сервере или страница «Доступ» (🔑 в шапке сайта). Службу перезапускать не
нужно — новый `server/.passhash` она подхватывает сама. Сессии на всех
устройствах закроются сами — билет подписан ключом, выведенным из хэша пароля.

---

## Pillar Gateway — локальный шлюз к моделям

Экран «Подключения» умеет ходить к LLM-провайдерам (Grok и др.), но юнит
хранилища закрыт на петлю (`IPAddressDeny=any`) — и это намеренно. Выход —
свой мини-шлюз [`../gateway/`](../gateway/) на той же машине: слушает только
`127.0.0.1:18789`, держит ключи провайдеров в `gateway/providers.json`
(0600, вне git) и ходит наружу сам. Хранилище видит только петлю, его юнит
не меняется. Настройка — [`GATEWAY.md`](GATEWAY.md).

---

## Памятка

| Действие | Команда |
|---|---|
| Логи хранилища | `journalctl -u pillar-vault -f` |
| Перезапустить хранилище | `sudo systemctl restart pillar-vault` |
| Обновить сейчас, не дожидаясь такта | `sudo systemctl start pillar-vault-deploy` |
| Что делал деплой | `journalctl -u pillar-vault-deploy -n 50` |
| Когда следующий такт | `systemctl list-timers pillar-vault-deploy` |
| Приостановить автодеплой | `sudo systemctl disable --now pillar-vault-deploy.timer` |
| Сменить пароль | `bash deploy/passwd.sh` |
| Проверить хранилище | `server/pillar-vault-server -root . -lint` |
| Проверить nginx | `sudo nginx -t && sudo systemctl reload nginx` |
| Обновить сертификат | certbot сам, проверка: `sudo certbot renew --dry-run` |
| Логи шлюза | `journalctl -u pillar-gateway -f` |
| Перезапустить шлюз | `sudo systemctl restart pillar-gateway` |

Публичного сада нет — ни витрины, ни механизма. Workflow `garden.yml` снят
2026-08-27, а 2026-09-07 из сервера убран и сам срез: флага `-build-public`
и файла `data.public.js` больше не существует, поле `public:` во frontmatter
ни на что не влияет и удалено. Без сессии сервер отдаёт только форму входа.
Если сад когда-нибудь понадобится, его придётся поднимать заново и осознанно.
