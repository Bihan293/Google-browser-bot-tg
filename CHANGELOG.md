# Changelog

All notable changes to this project will be documented in this file.

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
