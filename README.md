Сборка:

Компиляция исполняемого файла:
go build -o downloading-the-INN-extract.exe .

$root = "{Ваша директория с исполняемым файлом}\playwright"

New-Item -ItemType Directory -Force "$root\driver"
New-Item -ItemType Directory -Force "$root\browsers"

$env:PLAYWRIGHT_DRIVER_PATH = "$root\driver"
$env:PLAYWRIGHT_BROWSERS_PATH = "$root\browsers"

go run github.com/mxschmitt/playwright-go/cmd/playwright@v0.6100.0 install chromium

Запуск:
.\downloading-the-INN-extract.exe request.json
