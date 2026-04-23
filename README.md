# Eldorado Seller Bot (Go)

Bu proje, Eldorado.gg üzerinde Valorant boost satıcısı için:

- Açık siparişleri periyodik olarak kontrol eden,
- Uygun siparişlere otomatik teklif vermeyi hedefleyen,
- Teklif kabul edilip sipariş sana atandığında Telegram üzerinden bildirim gönderen

bir Go botunun iskeletini içerir.

> Not: Eldorado Seller API endpoint yolları ve bazı JSON alan adları, Swagger dokümanına göre senin tarafında güncellenmelidir. Kodda bu alanlar yorumlarla işaretlenmiştir.

## Çalıştırma

1. Gerekli ortam değişkenlerini ayarla (örn. bir `.env` dosyası ile veya direkt shell ortamında):

- `ELDORADO_BASE_URL`
- `ELDORADO_EMAIL`
- `ELDORADO_PASSWORD`
- `VALORANT_GAME_ID`
- `POLL_INTERVAL_OPEN_ORDERS`
- `POLL_INTERVAL_ORDER_STATUS`
- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_CHAT_ID`

2. Binary derle:

```bash
go build -buildvcs=false -o eldorado-bot ./cmd/bot
```

3. Botu çalıştır:

```bash
./eldorado-bot
```

İlk aşamada bot, Eldorado API endpoint ve alan isimleri doğru doldurulmuşsa:

- Açık Valorant siparişlerini tarar,
- Filtre kurallarına uyan siparişlere teklif vermeye çalışır,
- Teklifin kabulüyle sipariş sana atandığında Telegram bildirim gönderir.

