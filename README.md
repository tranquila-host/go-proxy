# Обход блокировки FingerprintManager в BrowserAutomationStudio

**Кратко:** из-за блокировок некоторых IP/диапазонов часть пользователей не может обращаться к `fingerprints.bablosoft.com` напрямую. Этот проект — простой прокси-ретранслятор на Go, который позволяет обойти блокировки: прокси сам запрашивает ресурс у `fingerprints.bablosoft.com` и возвращает финальный ответ клиенту (без пересылки 3xx редиректов на заблокированные IP).

> Written in Go. Предназначен для запуска на внешнем сервере, который не затронут блокировками (и не является заблокированным).

---
## Подходящие сервера можно купить тут, тех. поддержка бесплатно поможет с установкой: https://t.me/TranquilaHostbot

## Быстрый старт

1. Скопируй `main.go` (Go-код прокси) в каталог на сервере и собери/запусти.

**Пример запуска (быстро):**
```bash
go run main.go -target=fingerprints.bablosoft.com -listen=":80"
```

Или собрать бинарь:
```bash
go build -o fp-proxy main.go
./fp-proxy -target=fingerprints.bablosoft.com -listen=":9999"
```

> Рекомендуем запускать Go-прокси локально и пробрасывать его через nginx (пример ниже).

---

## Настройка клиента (hosts)

На клиенте (где запускается BrowserAutomationStudio) добавь в файл `hosts` строку:

**Windows**
```
C:\Windows\System32\drivers\etc\hosts
```

**Linux / macOS**
```
/etc/hosts
```

Добавь:
```
123.123.123.123    fingerprints.bablosoft.com
```
`123.123.123.123` — замените на IP вашего сервера (где запущен прокси или nginx).


---

## Рекомендуемая схема: nginx + Go

1. Запусти Go-прокси локально, например на `:9999`:
```bash
./fp-proxy -target=fingerprints.bablosoft.com -listen=":9999"
```

2. Пример минимальной конфигурации nginx (файл в `/etc/nginx/sites-available/default`):
```nginx
server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name _;

    access_log /var/log/nginx/fp.access.log;
    error_log  /var/log/nginx/fp.error.log;

    location / {
        proxy_pass http://127.0.0.1:9999;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_buffering off;
    }
}
```

3. Применить конфиг:
```bash
sudo nginx -t && sudo systemctl reload nginx
```

Теперь любой HTTP-запрос, пришедший на IP сервера или на `fingerprints.bablosoft.com` (если клиент сделал hosts), будет проксирован через nginx → Go → origin.

Если нужен HTTPS на nginx — добавьте соответствующие `listen 443 ssl` директивы и сертификаты.

---

## Что делает прокси

- Принимает запрос от клиента.
- Выполняет запрос к `fingerprints.bablosoft.com` (через Cloudflare / origin).
- Если origin возвращает 3xx (redirect) — прокси **сам вручную следует по `Location`** и получает конечный ответ (так клиент не увидит редиректы на IP, которые у него могут быть заблокированы).
- Возвращает клиенту полный финальный ответ (заголовки + тело).

---

## Безопасность — прочти обязательно

**Не используйте чужие публичные ретрансляторы для приватных/чувствительных запросов.**

- Оператор любого ретранслятора видит **весь** проходящий трафик: URL, параметры, заголовки, тело запросов и API-ключи.
- В запросах к FingerprintManager могут передаваться **API-ключи** и другие приватные данные. Пользование чужим прокси даёт третьей стороне возможность **перехватить и украсть** ваши данные или ключи.
- Рекомендации:
  - ВСЕГДА используйте **собственный сервер**, которому вы доверяете.
  - НЕ УКАЗЫВАЙТЕ публичные ретрансляторы.
  
---

## Почему НЕ стоит использовать публичный ретранслятор

- API Ключи могут украдены и Вы **НАВСЕГДА** утеряете к ним доступ, **БЕЗ ВОЗМОЖНОСТИ** восстановления, сервер стоит меньше 500 рублей в месяц, а лицензия FingerprintManager 3000 рублей. https://t.me/TranquilaHostbot

