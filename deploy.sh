#!/bin/bash

# Скрипт быстрого развертывания Telegram бота для УПД
# Использование: ./deploy.sh

set -e

echo "🚀 Развертывание Telegram бота для УПД в МойСклад"
echo "=================================================="

# Проверяем наличие Docker
if ! command -v docker &> /dev/null; then
    echo "❌ Docker не установлен. Установите Docker Desktop и повторите попытку."
    exit 1
fi

# Проверяем наличие docker-compose
if ! command -v docker-compose &> /dev/null; then
    echo "❌ docker-compose не установлен. Установите docker-compose и повторите попытку."
    exit 1
fi

# Проверяем, запущен ли Docker
if ! docker info &> /dev/null; then
    echo "❌ Docker не запущен. Запустите Docker Desktop и повторите попытку."
    exit 1
fi

echo "✅ Docker готов к работе"

# Проверяем наличие .env файла
if [ ! -f ".env" ]; then
    echo "📝 Создаю файл конфигурации .env..."
    cp .env.example .env
    echo "⚠️  ВАЖНО: Отредактируйте файл .env и заполните необходимые токены:"
    echo "   - TELEGRAM_BOT_TOKEN (получите у @BotFather)"
    echo "   - MOYSKLAD_API_TOKEN (получите в МойСклад → Настройки → API)"
    echo "   - AUTHORIZED_USERS (ваш Telegram ID от @userinfobot)"
    echo ""
    echo "После заполнения .env запустите скрипт снова."
    exit 0
fi

echo "✅ Файл конфигурации .env найден"

# Проверяем основные переменные
if grep -q "your_telegram_bot_token_here" .env; then
    echo "⚠️  Обнаружены незаполненные токены в .env файле."
    echo "   Заполните все необходимые параметры и запустите скрипт снова."
    exit 1
fi

echo "✅ Конфигурация выглядит корректно"

# Создаем необходимые директории
echo "📁 Создаю необходимые директории..."
mkdir -p logs temp data

echo "✅ Директории созданы"

# Останавливаем существующие контейнеры (если есть)
echo "🛑 Останавливаю существующие контейнеры..."
docker-compose down 2>/dev/null || true

# Собираем образ
echo "🔨 Собираю Docker образ..."
docker-compose build

# Запускаем бота
echo "🚀 Запускаю бота..."
docker-compose up -d

# Ждем немного для инициализации
echo "⏳ Ожидание инициализации..."
sleep 5

# Проверяем статус
echo "📊 Проверяю статус контейнера..."
if docker-compose ps | grep -q "Up"; then
    echo "✅ Бот успешно запущен!"
    echo ""
    echo "📋 Полезные команды:"
    echo "   docker-compose logs -f upd-telegram-bot  # Просмотр логов"
    echo "   docker-compose restart upd-telegram-bot  # Перезапуск"
    echo "   docker-compose down                      # Остановка"
    echo ""
    echo "🔗 Найдите вашего бота в Telegram и отправьте /start"
    echo "🔍 Логи доступны в директории: ./logs/"
    
    # Показываем последние логи
    echo ""
    echo "📄 Последние логи:"
    echo "=================="
    docker-compose logs --tail=10 upd-telegram-bot
    
else
    echo "❌ Ошибка запуска бота. Проверьте логи:"
    docker-compose logs upd-telegram-bot
    exit 1
fi

echo ""
echo "🎉 Развертывание завершено успешно!"