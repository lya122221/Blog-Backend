# 📝 Blog Backend

REST API бэкенд для IT-блога, написанный на **Go**. Поддерживает полный цикл работы с публикациями: авторизация пользователей, создание и редактирование статей с тегами, комментарии, лайки и подсчёт просмотров.

## 🛠 Технологический стек

| Категория | Технология |
|---|---|
| **Язык** | Go 1.25 |
| **HTTP-фреймворк** | [Gin](https://github.com/gin-gonic/gin) |
| **База данных** | PostgreSQL 15 |
| **Кэш / счётчики** | Redis 7 |
| **Аутентификация** | JWT (HS256) + bcrypt |
| **Миграции** | [golang-migrate](https://github.com/golang-migrate/migrate) |
| **Контейнеризация** | Docker & Docker Compose |

## 📐 Архитектура

Проект следует принципам **чистой архитектуры** с разделением на слои:

```
Blog-Backend/
├── cmd/blog/              # Точка входа приложения
│   └── main.go
├── internal/
│   ├── handlers/          # HTTP-обработчики (транспортный слой)
│   ├── services/          # Бизнес-логика
│   ├── repositories/      # Работа с БД и Redis (слой данных)
│   ├── models/            # Структуры данных
│   ├── middleware/         # JWT-аутентификация
│   └── workers/           # Фоновые задачи
├── pkg/                   # Утилиты (генерация JWT)
├── migrations/            # SQL-миграции
├── Dockerfile
└── docker-compose.yml
```

Счётчик просмотров статей реализован через **Redis + фоновый воркер**: при каждом запросе статьи инкрементируется значение в Redis, а воркер раз в 3 минуты сбрасывает накопленные значения в PostgreSQL. Это снижает нагрузку на основную базу данных.

## 🗄 Схема базы данных

```
┌──────────────┐       ┌───────────────┐       ┌──────────────┐
│    users     │       │   articles    │       │     tags     │
├──────────────┤       ├───────────────┤       ├──────────────┤
│ id       (PK)│◄──────│ author_id (FK)│       │ id       (PK)│
│ username     │       │ id        (PK)│──┐    │ name  (UNIQ) │
│ email        │       │ title         │  │    └──────┬───────┘
│ password_hash│       │ content       │  │           │
│ created_at   │       │ views_count   │  │    ┌──────┴───────┐
│ bio          │       │ created_at    │  │    │ article_tags │
└──────┬───────┘       └───────┬───────┘  │    ├──────────────┤
       │                       │          ├───►│ article_id   │
       │                       │          │    │ tag_id       │
       │               ┌──────┴───────┐  │    └──────────────┘
       │               │   comments   │  │
       │               ├──────────────┤  │
       └──────────────►│ user_id  (FK)│  │
                       │ article_id(FK)│◄─┘
                       │ id        (PK)│
                       │ content       │
                       │ created_at    │
                       └──────────────┘

       ┌──────────────┐
       │    likes     │
       ├──────────────┤
       │ article_id   │◄── articles.id
       │ user_id      │◄── users.id
       │ (PK: оба)    │
       └──────────────┘
```

## 🔌 API-эндпоинты

Базовый путь: `/api/v1`

### Аутентификация

| Метод | Путь | Описание |
|---|---|---|
| `POST` | `/auth/register` | Регистрация нового пользователя |
| `POST` | `/auth/login` | Вход и получение JWT-токена |

### Статьи

| Метод | Путь | Авторизация | Описание |
|---|---|---|---|
| `GET` | `/articles/` | ❌ | Получить список статей (с пагинацией и фильтрацией по тегам) |
| `GET` | `/articles/:id` | ❌ | Получить статью по ID |
| `POST` | `/articles/` | ✅ | Создать новую статью |
| `PUT` | `/articles/:id` | ✅ | Обновить статью (только автор) |
| `DELETE` | `/articles/:id` | ✅ | Удалить статью (только автор) |

### Взаимодействия

| Метод | Путь | Авторизация | Описание |
|---|---|---|---|
| `GET` | `/articles/:id/comments` | ❌ | Получить комментарии к статье |
| `POST` | `/articles/:id/comments` | ✅ | Оставить комментарий |
| `POST` | `/articles/:id/like` | ✅ | Поставить / снять лайк (toggle) |

### Параметры запросов

**GET `/articles/`**:
- `page` — номер страницы (по умолчанию: `1`)
- `limit` — количество статей на странице (по умолчанию: `20`, макс: `99`)
- `tag` — фильтр по тегу (можно указать несколько: `?tag=go&tag=docker`)

## 🚀 Запуск

### Предварительные требования

- [Docker](https://docs.docker.com/get-docker/) и [Docker Compose](https://docs.docker.com/compose/install/)

### 1. Клонировать репозиторий

```bash
git clone https://github.com/lya122221/Blog-Backend.git
cd Blog-Backend
```

### 2. Настроить переменные окружения

Создайте файл `.env` в корне проекта:

```env
POSTGRES_USER=your_user
POSTGRES_PASSWORD=your_password
POSTGRES_DB=blog_db
JWTKEY=your_secret_jwt_key
```

### 3. Запустить

```bash
docker compose up --build
```

Сервис будет доступен на `http://localhost:8080`.

Docker Compose автоматически:
- поднимет **PostgreSQL** и **Redis**
- применит все **миграции**
- запустит **API-сервер**

## 📋 Примеры запросов

### Регистрация

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com", "username": "user", "password": "secret123"}'
```

### Логин

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com", "password": "secret123"}'
```

### Создание статьи

```bash
curl -X POST http://localhost:8080/api/v1/articles/ \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <TOKEN>" \
  -d '{
    "title": "Начало работы с Go",
    "content": "Go — компилируемый язык программирования...",
    "tags": ["go", "tutorial"]
  }'
```

### Получение статей с фильтрацией

```bash
curl "http://localhost:8080/api/v1/articles/?page=1&limit=10&tag=go"
```

### Лайк статьи

```bash
curl -X POST http://localhost:8080/api/v1/articles/<ARTICLE_ID>/like \
  -H "Authorization: Bearer <TOKEN>"
```
