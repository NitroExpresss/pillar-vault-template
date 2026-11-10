# Pillar Gateway — локальный шлюз к моделям

Экран «Подключения» ходит к LLM-провайдерам с машины сервера, но юнит
`pillar-vault.service` закрыт на петлю (`IPAddressDeny=any`) — наружу он
ходить не может и не должен. Выход — свой мини-шлюз [`../gateway/`](../gateway/)
на той же машине: слушает только `127.0.0.1:18789`, держит ключи провайдеров в
`gateway/providers.json` (0600, вне git) и пересылает OpenAI-совместимые
запросы (`/v1/models`, `/v1/chat/completions`) апстриму. Хранилище видит
только петлю — его юнит не трогается.

```
приложение → pillar-vault (петля) → pillar-gateway 127.0.0.1:18789 → api.x.ai
                                      ключи провайдеров здесь,
                                      в gateway/providers.json
```

Всё выполняется на VDS под тем же пользователем, что владеет
`/opt/pillar-vault` (в примерах — `deploy`).

---

## Установка

```bash
cd /opt/pillar-vault
/usr/local/go/bin/go build -trimpath -o gateway/pillar-gateway ./gateway
sudo -u deploy cp gateway/providers.example.json gateway/providers.json
chmod 600 gateway/providers.json
nano gateway/providers.json                 # вписать ключ xAI
sudo cp deploy/pillar-gateway.service /etc/systemd/system/
sudo sed -i "s/^User=.*/User=$USER/;s/^Group=.*/Group=$(id -gn)/" \
     /etc/systemd/system/pillar-gateway.service
sudo systemctl daemon-reload
sudo systemctl enable --now pillar-gateway
```

## Конфиг providers.json

```json
{
  "default": "grok",
  "providers": {
    "grok": {
      "base_url": "https://api.x.ai/v1",
      "key": "xai-...",
      "models": ["grok-4.7", "grok-4.3"]
    }
  }
}
```

Несколько провайдеров — несколько записей под своими именами. Модель
«имя/модель» в запросе ищет провайдера по имени, голое имя модели — по
спискам моделей, иначе — провайдер `default`. Файл перечитывается при
изменении: смена ключа перезапуска не требует.

## Проверка

```bash
curl -sS http://127.0.0.1:18789/health           # ok
curl -sS http://127.0.0.1:18789/v1/models        # модели из конфига
curl -sS http://127.0.0.1:18789/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"grok-4.7","messages":[{"role":"user","content":"привет"}]}'
```

В приложении: **Подключения → добавить → «Шлюз (локальный)»** →
«Проверить ключ» / «Спросить модель».

## Памятка

| Действие | Команда |
|---|---|
| Логи шлюза | `journalctl -u pillar-gateway -f` |
| Перезапустить шлюз | `sudo systemctl restart pillar-gateway` |
| Пересобрать после правок в `gateway/` | `cd /opt/pillar-vault && /usr/local/go/bin/go build -trimpath -o gateway/pillar-gateway ./gateway && sudo systemctl restart pillar-gateway` |

⚠ Шлюз слушает только петлю — сам бинарь откажется стартовать на внешнем
адресе. Наружу его не выставлять никогда: авторизации нет, кто достучался,
тот тратит ключи.
