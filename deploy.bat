@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion

echo 🚀 Развертывание Telegram бота для УПД в МойСклад
echo ==================================================

REM Проверяем наличие Docker
docker --version >nul 2>&1
if errorlevel 1 (
    echo ❌ Docker не установлен. Установите Docker Desktop и повторите попытку.
    pause
    exit /b 1
)

REM Проверяем наличие docker-compose
docker-compose --version >nul 2>&1
if errorlevel 1 (
    echo ❌ docker-compose не установлен. Установите docker-compose и повторите попытку.
    pause
    exit /b 1
)

REM Проверяем, запущен ли Docker
docker info >nul 2>&1
if errorlevel 1 (
    echo ❌ Docker не запущен. Запустите Docker Desktop и повторите попытку.
    pause
    exit /b 1
)

echo ✅ Docker готов к работе

REM Проверяем наличие .env файла
if not exist ".env" (
    echo 📝 Создаю файл конфигурации .env...
    copy .env.example .env >nul
    echo ⚠️  ВАЖНО: Отредактируйте файл .env и заполните необходимые токены:
    echo    - TELEGRAM_BOT_TOKEN ^(получите у @BotFather^)
    echo    - MOYSKLAD_API_TOKEN ^(получите в МойСклад → Настройки → API^)
    echo    - AUTHORIZED_USERS ^(ваш Telegram ID от @userinfobot^)
    echo.
    echo После заполнения .env запустите скрипт снова.
    pause
    exit /b 0
)

echo ✅ Файл конфигурации .env найден

REM Проверяем основные переменные
findstr /C:"your_telegram_bot_token_here" .env >nul 2>&1
if not errorlevel 1 (
    echo ⚠️  Обнаружены незаполненные токены в .env файле.
    echo    Заполните все необходимые параметры и запустите скрипт снова.
    pause
    exit /b 1
)

echo ✅ Конфигурация выглядит корректно

REM Создаем необходимые директории
echo 📁 Создаю необходимые директории...
if not exist "logs" mkdir logs
if not exist "temp" mkdir temp
if not exist "data" mkdir data

echo ✅ Директории созданы

REM Останавливаем существующие контейнеры (если есть)
echo 🛑 Останавливаю существующие контейнеры...
docker-compose down >nul 2>&1

REM Собираем образ
echo 🔨 Собираю Docker образ...
docker-compose build
if errorlevel 1 (
    echo ❌ Ошибка сборки образа
    pause
    exit /b 1
)

REM Запускаем бота
echo 🚀 Запускаю бота...
docker-compose up -d
if errorlevel 1 (
    echo ❌ Ошибка запуска бота
    pause
    exit /b 1
)

REM Ждем немного для инициализации
echo ⏳ Ожидание инициализации...
timeout /t 5 /nobreak >nul

REM Проверяем статус
echo 📊 Проверяю статус контейнера...
docker-compose ps | findstr "Up" >nul 2>&1
if not errorlevel 1 (
    echo ✅ Бот успешно запущен!
    echo.
    echo 📋 Полезные команды:
    echo    docker-compose logs -f upd-telegram-bot  # Просмотр логов
    echo    docker-compose restart upd-telegram-bot  # Перезапуск
    echo    docker-compose down                      # Остановка
    echo.
    echo 🔗 Найдите вашего бота в Telegram и отправьте /start
    echo 🔍 Логи доступны в директории: ./logs/
    
    REM Показываем последние логи
    echo.
    echo 📄 Последние логи:
    echo ==================
    docker-compose logs --tail=10 upd-telegram-bot
    
) else (
    echo ❌ Ошибка запуска бота. Проверьте логи:
    docker-compose logs upd-telegram-bot
    pause
    exit /b 1
)

echo.
echo 🎉 Развертывание завершено успешно!
pause