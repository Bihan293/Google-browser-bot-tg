# Changelog

All notable changes to this project will be documented in this file.

## [0.3.0] - 2026-05-11

### Fixed — сборка в Docker и кнопка «Новости»
- **Сборка Docker-образа больше не падает.** `tgbotapi.VideoConfig`
  не имеет полей `Width`/`Height` (Telegram сам выводит их из контейнера
  mp4), убраны нерабочие присваивания в `internal/handlers/open_ops.go` —
  ошибка `vid.Width undefined (type tgbotapi.VideoConfig has no field or method Width)`
  устранена.
- **Кнопка «📰 Новости» теперь открывает реальные статьи.** Раньше из RSS
  Google News приходила ссылка вида
  `https://news.google.com/rss/articles/...`, бот пытался отрендерить
  именно её и видел только заглушку «Comprehensive up-to-date news
  coverage…». Теперь:
  - сначала пробуем извлечь настоящий `<a href>` издателя из
    description-блока RSS-итема;
  - если там тоже Google — следуем редиректу (`Probe` → финальный URL),
    с фолбэком на парсинг HTML-страницы Google News;
  - кэшируем разрешённый URL в памяти, чтобы не платить за повторные
    round-trips;
  - локаль RSS теперь подбирается по запросу: кириллица → `hl=ru&gl=RU&ceid=RU:ru`,
    иначе — английская выдача;
  - в карточке новости показывается хост издателя (`bbc.com`, `lenta.ru`…),
    а не «Google News».

### Added — /ping и health-check
- **Команда `/ping`** (и кнопка `📡 Пинг` в главном меню) — бот сам
  стучится в свой публичный `WEBHOOK_URL/health`, измеряет RTT,
  проверяет Telegram API и отвечает «🏓 pong — ok, понял!» с метриками
  (uptime, HTTP-статус, тело ответа).
- **HTTP-эндпоинты:** `/health` (JSON), `/healthz` (синоним для
  Kubernetes-style проверок), `/ping` (plain `pong`) и корневой `/`
  с короткой заглушкой.
- **Команды `/clear` / `/reset`** — сбросить сессию пользователя.

### Changed — UX и стабильность
- Стартовое и help-сообщения переписаны, добавлены эмодзи и группировка
  команд.
- Главное меню получило кнопку `📡 Пинг`.
- Длинные статьи теперь корректно режутся на части ≤ 3500 рун с разрывом
  по абзацу/предложению — больше не падают из-за Telegram-лимита в 4096
  символов.
- `sendArticleHeader` отделяет шапку (title + URL + host + description)
  от тела статьи и шлёт их отдельными сообщениями.
- В новостях:
  - убран двойной разделитель `«• »` при пустом источнике/дате;
  - дата RSS приводится к читабельному `02 Jan 2006, 15:04 UTC`;
  - из заголовка убирается хвост `« - Source»`, который Google клеит сам.
- Callback-обработчик защищён от `nil` `cb.Message` / `cb.Data == ""`.
- `escapeAttr` помечен `var _ = escapeAttr`, чтобы не падал линтер на
  «unused» при сохранении функции для будущего использования.

## [0.2.0] - 2026-05-11

### Changed — "Браузер реально работает"
- **Все ссылки в результатах поиска и на открытых страницах теперь — это
  inline-кнопки**, а не HTML-якоря. По нажатию бот открывает страницу
  внутри Telegram, никаких внешних переходов.
- **Открытие страницы** теперь вытаскивает не только текст: бот собирает
  все `<img>`, `<video>`/`<source>`, OG-изображения, `srcset`/lazy-src,
  YouTube/Vimeo iframe-эмбеды и обычные `<a href>`, и шлёт всё это
  в чат:
  - картинки — альбомами (media group до 10),
  - видео — пытаемся реально проиграть (см. ниже),
  - ссылки — кнопками внизу под страницей, с пагинацией.
- **Видео реально проигрывается в чате.** Для YouTube/Shorts/youtu.be бот
  обходит публичные Invidious-инстансы, тянет mp4-поток (до 50 МБ —
  ограничение Bot API) и шлёт через `sendVideo` с поддержкой streaming.
  Раньше выдавалась только ссылка на YouTube.
- **Превью видео-результатов теперь альбомом**, как у картинок —
  визуально куда удобнее.
- **18+ контент включается кнопкой `🔞 18+`** или командой `/nsfw`.
  Под капотом — отключение SafeSearch у DuckDuckGo (`p=-2`, cookie
  `kp=-2`) и у YouTube (cookie `PREF=f2=8000000`).
- HTTP-клиент получил cookies-jar, методы `Probe` и `Download`,
  увеличил лимит ответа до 6 МБ и тайм-аут до 20 с.

### Added
- `internal/search/youtube.go` — YouTubeFetcher через Invidious.
- `OpenedPageKeyboard` — клавиатура под открытой страницей со ссылками,
  «Все фото», «Видео» и пагинацией.
- Команда `/nsfw` и кнопка `🔞 18+`.
- В сессии — кэш «коротких ID → URL» (`LinkMap`), чтобы любые длинные
  ссылки умещались в callback_data (64 байта).

## [0.1.0] - 2026-05-11

### Added
- Initial release of `tg-browser-bot`.
- Telegram webhook server with health-check (`/healthz`).
- Search providers (no API keys required).
- In-chat page renderer.
- Inline keyboards.
- In-memory session store.
