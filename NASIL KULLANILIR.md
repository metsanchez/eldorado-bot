# Eldorado Seller Bot — Kurulum ve Kullanım Kılavuzu

Eldorado.gg üzerinde Valorant boost siparişlerini otomatik olarak takip eden, teklif veren, Telegram bildirimi gönderen ve alıcıya otomatik mesaj atan bir Go botu.

---

## Gereksinimler

| Araç | Minimum Versiyon | Açıklama |
|------|-------------------|----------|
| **Go** | 1.24+ | Bot derlemesi için |
| **Python** | 3.9+ | Tarayıcı otomasyonu scriptleri için |
| **Google Chrome** | Güncel | patchright tarafından kullanılır |
| **patchright** | Son sürüm | Cloudflare bypass'lı Playwright fork'u |
| **macOS / Linux** | — | Windows test edilmedi |

---

## 1. Go Kurulumu

### macOS (Homebrew)

```bash
brew install go
```

### Linux

```bash
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

Doğrulama:

```bash
go version
# go version go1.24.x darwin/arm64
```

---

## 2. Python ve patchright Kurulumu

### Python Kurulumu (macOS)

```bash
brew install python3
```

### patchright Kurulumu

```bash
pip3 install patchright
```

patchright'ın Chromium sürücüsünü indir:

```bash
python3 -m patchright install chromium
```

> **Not:** Bot, sisteminizde yüklü olan Google Chrome'u (`channel="chrome"`) kullanır. Chrome'un yüklü ve güncel olduğundan emin olun.

Doğrulama:

```bash
python3 -c "from patchright.sync_api import sync_playwright; print('patchright OK')"
```

---

## 3. Projeyi İndirme ve Bağımlılıkları Yükleme

```bash
# Proje klasörüne git
cd "Eldorado Boyt"

# Go bağımlılıklarını indir
go mod download
```

---

## 4. Yapılandırma (.env Dosyası)

Proje kök dizininde `.env.example` dosyasını `.env` olarak kopyalayın ve düzenleyin:

```bash
cp .env.example .env
```

### .env Dosyası Açıklamaları

```bash
# ═══════════════════════════════════════════════════
# ELDORADO GİRİŞ BİLGİLERİ
# ═══════════════════════════════════════════════════

ELDORADO_BASE_URL=https://www.eldorado.gg
ELDORADO_EMAIL=senin_mailin@ornek.com        # Eldorado hesap e-posta adresi
ELDORADO_PASSWORD=buraya_sifreni_yaz          # Eldorado hesap şifresi

# Opsiyonel: Manuel cookie/token (boş bırakılırsa otomatik tarayıcı login yapılır)
ELDORADO_COOKIES=
ELDORADO_XSRF_TOKEN=

VALORANT_GAME_ID=32                           # Valorant oyun ID'si (değiştirmeyin)

# ═══════════════════════════════════════════════════
# POLLING ARALIKLARI
# ═══════════════════════════════════════════════════

POLL_INTERVAL_OPEN_ORDERS=30s                 # Yeni sipariş kontrolü (örn: 30s, 1m)
POLL_INTERVAL_ORDER_STATUS=30s                # Teklif durumu kontrolü

# ═══════════════════════════════════════════════════
# TELEGRAM BİLDİRİMLERİ
# ═══════════════════════════════════════════════════

TELEGRAM_BOT_TOKEN=123456789:AAxxxxxxxxxxxxxxx  # BotFather'dan aldığın token
TELEGRAM_CHAT_ID=-100xxxxxxxxx                   # Grup/Kanal chat ID

# ═══════════════════════════════════════════════════
# TEKLİF AYARLARI
# ═══════════════════════════════════════════════════

MIN_OFFER_PRICE=3                             # Minimum teklif fiyatı ($)
MAX_OFFER_PRICE=0                             # 0 = limit yok
OFFER_MESSAGE=Merhaba, hızlı ve güvenli boost yapabilirim.
DELIVERY_TIME=Hour3                           # Varsayılan teslimat süresi

# ═══════════════════════════════════════════════════
# OTOMATİK ALICI MESAJI
# ═══════════════════════════════════════════════════

# Teklif verildikten sonra alıcıya otomatik gönderilecek mesaj
# Satır sonu için \n kullanın
BUYER_AUTO_MESSAGE=Hey! 👋 Radiant Top #2 player here.\n\nI personally handle every boost with the highest win rates and fastest completion on the platform.\n\n🏆 100% Win Rate Record\n⚡ Lightning-Fast Delivery\n🔒 Full Account Security (VPN + Offline Mode)\n\n✅ Free Live Stream\n✅ Free Agent Selection\n✅ Free Priority Queue\n\nI treat every account like my own.\nLet's get you to your dream rank — fast, safe, and guaranteed. 💎

# Mesajla birlikte gönderilecek görsel (proje kök dizinine koyun)
BUYER_AUTO_IMAGE=Radifix.jpeg
```

---

## 5. Telegram Bot Kurulumu

### 5.1 Bot Oluşturma

1. Telegram'da [@BotFather](https://t.me/BotFather) ile konuşma başlatın
2. `/newbot` komutunu gönderin
3. Bot adı ve kullanıcı adı belirleyin
4. BotFather size bir **token** verecek → `.env` dosyasındaki `TELEGRAM_BOT_TOKEN` alanına yapıştırın

### 5.2 Chat ID Alma

**Kişisel mesaj için:**

1. Oluşturduğunuz bota `/start` gönderin
2. Tarayıcıda şu URL'yi açın (TOKEN'ı kendi tokenınızla değiştirin):
   ```
   https://api.telegram.org/botTOKEN/getUpdates
   ```
3. JSON çıktısında `"chat":{"id": 123456789}` değerini bulun
4. Bu sayıyı `TELEGRAM_CHAT_ID` alanına yazın

**Grup için:**

1. Botu gruba ekleyin
2. Grupta bir mesaj gönderin
3. Aynı `getUpdates` URL'sini açın
4. Grup chat ID'si negatif bir sayı olacaktır (örn: `-100123456789`)

---

## 6. Otomatik Mesaj Görseli

Alıcıya mesajla birlikte görsel göndermek istiyorsanız:

1. Görsel dosyasını (örn: `Radifix.jpeg`) proje kök dizinine koyun
2. `.env` dosyasında `BUYER_AUTO_IMAGE=Radifix.jpeg` olarak ayarlayın
3. Görsel göndermek istemiyorsanız bu satırı boş bırakın: `BUYER_AUTO_IMAGE=`

> **Desteklenen formatlar:** JPEG, PNG

---

## 7. Botu Derleme ve Çalıştırma

### Yöntem 1: start.sh ile (Önerilen)

```bash
chmod +x start.sh
./start.sh
```

Bu komut:
- Çalışan eski bot süreçlerini otomatik olarak sonlandırır
- Botu yeniden derler
- Yeni derlenen botu başlatır

### Yöntem 2: Manuel

```bash
# Derleme
go build -o eldorado-bot ./cmd/bot/

# Çalıştırma
./eldorado-bot
```

### Yöntem 3: go run ile (Geliştirme)

```bash
go run ./cmd/bot/
```

---

## 8. İlk Çalıştırma — Ne Olur?

Bot ilk kez çalıştırıldığında şu adımlar gerçekleşir:

```
1. .env dosyası yüklenir
2. Config doğrulanır (email+password veya cookies zorunlu)
3. Tarayıcı login başlatılır:
   - Chrome açılır (görünür pencere)
   - Eldorado.gg'ye gidilir
   - Cloudflare challenge çözülür
   - E-posta ve şifre girilir
   - Oturum cookie'leri kaydedilir → storage/browser_cookies.json
4. İki polling döngüsü başlar:
   a. Yeni sipariş tarama (her 30 saniye)
   b. Teklif durumu kontrolü (her 30 saniye)
```

> **İlk login sırasında Chrome penceresi açılacaktır.** Bu normaldir. Cloudflare challenge otomatik çözülür. Pencereye müdahale etmeyin.

---

## 9. Bot Nasıl Çalışır?

### Sipariş Tarama Döngüsü

```
Her 30 saniyede:
  → Eldorado API'den aktif boost taleplerini çek
  → Her yeni talep için:
      1. Daha önce görüldü mü? (storage.json kontrol)
      2. Filtreler:
         - Sadece EU sunucu ✓
         - Sadece Rank Boost ve Net Wins kategorileri ✓
         - Radiant boost → atla ✗
         - Immortal 300+ RR → atla ✗
         - Custom/Placement/Coaching → atla ✗
      3. Fiyat hesapla (pricing.go kurallarına göre)
      4. Teklif oluştur
      5. Telegram bildirimi gönder
      6. Alıcıya otomatik mesaj + görsel gönder (tarayıcı üzerinden)
```

### Teklif Durumu Kontrolü

```
Her 30 saniyede:
  → Kazanılan teklifleri kontrol et → "Sipariş atandı!" bildirimi
  → Kaybedilen teklifleri kontrol et → durumu güncelle
```

---

## 10. Fiyatlandırma Tablosu

### RR Bazlı Fiyatlandırma (Temel Veri)

Bot, fiyatları tier başına **ortalama RR kazanımı** ve **oyun başı fiyat** üzerinden hesaplar:

| Rank Tier | RR / Oyun | Fiyat / Oyun ($) |
|-----------|-----------|------------------|
| Iron | 22 | 1 |
| Bronze | 22 | 1 |
| Silver | 22 | 1 |
| Gold | 22 | 2 |
| Platinum | 22 | 3 |
| Diamond | 20 | 4 |
| Ascendant | 18 | 5 |
| Immortal | 17 | 10 |

### Rank Boost — Hesaplama Mantığı

Her division 100 RR'dır. Bir division'ı tamamlamak için gereken oyun sayısı:

```
Oyun sayısı = ceil(gerekli_RR / RR_per_oyun)
Division maliyeti = oyun sayısı × oyun başı fiyat
```

**İlk division'da mevcut RR hesaba katılır:**

- Oyuncu 80 RR'daysa → sadece 20 RR gerekir → daha az oyun → daha ucuz
- Sonraki divisionlar tam 100 RR olarak hesaplanır

**Örnek:** Iron I (80 RR) → Bronze I boost:

| Division | Gereken RR | Oyun | Fiyat |
|----------|-----------|------|-------|
| Iron I → Iron II | 20 (100-80) | ceil(20/22) = 1 | $1 |
| Iron II → Iron III | 100 | ceil(100/22) = 5 | $5 |
| Iron III → Bronze I | 100 | ceil(100/22) = 5 | $5 |
| **Toplam** | | | **$11** |

**Tam division fiyatları (0 RR'dan):**

| Rank Tier | Oyun/Division | Fiyat/Division ($) |
|-----------|---------------|-------------------|
| Iron | 5 | 5 |
| Bronze | 5 | 5 |
| Silver | 5 | 5 |
| Gold | 5 | 10 |
| Platinum | 5 | 15 |
| Diamond | 5 | 20 |
| Ascendant | 6 | 30 |
| Immortal | 6 | 60 |

> **Duo** sipariş = toplam fiyat **x2**
> Teslimat süresi: Iron–Plat division başı 4 saat, Ascendant ve sonrası division başı 7 saat

### Net Win (Oyun Başına Fiyat)

| Rank Tier | Fiyat ($) |
|-----------|-----------|
| Iron — Platinum | 5 |
| Diamond | 5 |
| Ascendant | 8 |
| Immortal I | 11 |

> **Duo** sipariş = fiyat **x2**

### Point (RR Bazlı Siparişler)

Oyun başı fiyat aynı tablodan alınır:

| Rank Tier | Fiyat / Oyun ($) |
|-----------|------------------|
| Iron | 1 |
| Bronze | 1 |
| Silver | 1 |
| Gold | 2 |
| Platinum | 3 |
| Diamond | 4 |
| Ascendant | 5 |
| Immortal | 10 |

---

## 11. Dosya ve Klasör Yapısı

```
Eldorado Boyt/
├── cmd/bot/main.go              # Giriş noktası
├── internal/
│   ├── config/config.go         # .env yükleyici ve doğrulayıcı
│   ├── eldorado/
│   │   ├── client.go            # Eldorado API istemcisi (curl ile)
│   │   ├── auth.go              # Kimlik doğrulama mantığı
│   │   ├── browser_login.go     # Python scriptleriyle köprü
│   │   └── models.go            # API veri modelleri
│   ├── logic/
│   │   ├── runner.go            # Ana bot döngüsü
│   │   ├── pricing.go           # Fiyat hesaplama
│   │   ├── matcher.go           # Sipariş filtreleme
│   │   └── retry.go             # Yeniden deneme mantığı
│   ├── storage/storage.go       # JSON dosya depolama
│   ├── telegram/bot.go          # Telegram bildirim gönderici
│   └── logger/logger.go         # Loglama
├── scripts/
│   ├── browser_login.py         # Tarayıcı ile Eldorado login
│   └── send_chat_message.py     # Tarayıcı ile alıcıya mesaj gönderme
├── storage/
│   └── browser_cookies.json     # Kaydedilmiş oturum cookie'leri
├── .env                         # Yapılandırma (gizli, git'e eklenmez)
├── .env.example                 # Örnek yapılandırma
├── storage.json                 # Görülen siparişler ve takip durumu
├── start.sh                     # Başlatma scripti
├── Radifix.jpeg                 # Otomatik mesaj görseli
└── go.mod                       # Go bağımlılıkları
```

---

## 12. Sık Kullanılan Komutlar

| Komut | Açıklama |
|-------|----------|
| `./start.sh` | Botu derle ve başlat (eski süreçleri öldürür) |
| `go build -o eldorado-bot ./cmd/bot/` | Sadece derle |
| `./eldorado-bot` | Derlenmiş botu çalıştır |
| `pkill -f eldorado-bot` | Botu durdur |
| `cat storage.json \| python3 -m json.tool` | Görülen siparişleri göster |
| `rm storage.json` | Tüm görülen siparişleri sıfırla |
| `rm storage/browser_cookies.json` | Oturum cookie'lerini sıfırla (yeniden login gerekir) |

---

## 13. VPS Kurulumu (xvfb ile)

Botu GUI olmayan bir VPS'de çalıştırırken Chrome için sanal ekran gerekir:

```bash
# xvfb kurulumu (Debian/Ubuntu)
sudo apt install xvfb

# Chrome kurulumu
wget -q -O - https://dl.google.com/linux/linux_signing_key.pub | sudo apt-key add -
echo "deb [arch=amd64] http://dl.google.com/linux/chrome/deb/ stable main" | sudo tee /etc/apt/sources.list.d/google-chrome.list
sudo apt update && sudo apt install google-chrome-stable
```

**Önerilen:** `start.sh` Linux’ta `DISPLAY` boşken (tipik VPS) otomatik olarak `xvfb-run -a` kullanır; önce `sudo apt install -y xvfb` kurun, sonra `./start.sh`.

Manuel tek komut:

```bash
xvfb-run -a ./eldorado-bot
```

Eski yöntem (elle Xvfb):

```bash
Xvfb :99 -screen 0 1920x1080x24 &
export DISPLAY=:99
./eldorado-bot
```

Masaüstünde yine de sanal ekran istiyorsanız: `ELDORADO_XVFB=1 ./start.sh`

> **Not:** Mesaj gönderme, xvfb ortamında `type()`/`press()` yerine JavaScript ile çalışacak şekilde ayarlandı. Eğer mesajlar hâlâ gitmiyorsa, `HEADLESS=1 ./eldorado-bot` ile deneyebilirsin.

---

## 14. Sorun Giderme

### "either ELDORADO_COOKIES or both ELDORADO_EMAIL+ELDORADO_PASSWORD must be set"

`.env` dosyasında `ELDORADO_EMAIL` ve `ELDORADO_PASSWORD` alanlarının dolu olduğundan emin olun.

### Chrome penceresi açılıyor ama login olmuyor

- Google Chrome'un yüklü ve güncel olduğunu kontrol edin
- patchright'ın düzgün kurulduğunu doğrulayın:
  ```bash
  python3 -c "from patchright.sync_api import sync_playwright; print('OK')"
  ```
- Chrome penceresine müdahale etmeyin, Cloudflare challenge'ı otomatik çözülür

### Bot teklif vermiyor

1. `storage.json` dosyasını kontrol edin — siparişler "seen" olarak işaretlenmiş olabilir
2. Sıfırlamak için: `rm storage.json`
3. Mevcut aktif EU Valorant siparişi olduğundan emin olun
4. Log çıktısında "skipping" mesajlarını kontrol edin (filtre nedeni yazar)

### Kritik hata bildirimleri (Telegram)

Bot, aşağıdaki durumlarda Telegram’a uyarı gönderir:
- **Login hatası:** Bot başlarken Eldorado girişi başarısız olursa
- **API/Auth hatası:** 401/403 veya re-login 3 kez üst üste başarısız olursa
- **Mesaj hatası:** Alıcıya mesaj gönderme 3 kez üst üste başarısız olursa

Aynı türde tekrar uyarı gönderilmez; en az 1 saat geçmesi gerekir (spam önleme).

### Telegram bildirimi gelmiyor

1. `TELEGRAM_BOT_TOKEN` ve `TELEGRAM_CHAT_ID` değerlerinin doğru olduğunu kontrol edin
2. Bot gruba/kanala ekliyse, bota admin yetkisi verildiğinden emin olun
3. Test:
   ```bash
   curl "https://api.telegram.org/botTOKEN/sendMessage?chat_id=CHAT_ID&text=test"
   ```

### Alıcıya mesaj gitmiyor

1. `.env` dosyasında `BUYER_AUTO_MESSAGE` alanının dolu olduğunu kontrol edin
2. `storage/browser_cookies.json` dosyasının var olduğunu kontrol edin
3. **VPS'de:** `./start.sh` (içinde `xvfb-run -a`) veya `xvfb-run -a ./eldorado-bot`; `xvfb` paketi kurulu olmalı
4. Cookie'ler geçersiz olmuş olabilir — silin ve botu yeniden başlatın:
   ```bash
   rm storage/browser_cookies.json
   ./start.sh
   ```

### "HTTP 403 — XSRF cookie tokens are missing"

Oturum cookie'leri geçersiz olmuş. Yeniden login gerekli:

```bash
rm storage/browser_cookies.json
./start.sh
```

### Birden fazla Chrome penceresi açılıyor

Bot'un birden fazla kopyası çalışıyor olabilir:

```bash
# Tüm bot süreçlerini öldür
pkill -9 -f eldorado-bot
pkill -9 -f browser_login.py
pkill -9 -f send_chat_message.py

# Temiz başlat
./start.sh
```

---

## 15. Güvenlik Notları

- `.env` dosyası **asla** git'e eklenmez (`.gitignore`'da tanımlı)
- `storage/browser_cookies.json` oturum bilgilerinizi içerir — **paylaşmayın**
- `storage.json` sipariş geçmişinizi içerir
- Bot, hesabınızla tüm işlemleri otomatik yapar — **güvenilir bir ortamda çalıştırın**

---

## 16. Botu Durdurmak

```bash
# Yöntem 1: Ctrl+C ile (terminal'de çalışıyorsa)
# Yöntem 2:
pkill -f eldorado-bot

# Tüm ilgili süreçleri temizlemek için:
pkill -9 -f eldorado-bot
pkill -9 -f browser_login.py
pkill -9 -f send_chat_message.py
```

---

## Hızlı Başlangıç (Özet)

```bash
# 1. Gereksinimleri kur
brew install go python3
pip3 install patchright
python3 -m patchright install chromium

# 2. .env dosyasını oluştur
cp .env.example .env
# .env dosyasını düzenle: email, şifre, telegram bilgileri

# 3. Go bağımlılıklarını indir
go mod download

# 4. Botu başlat
chmod +x start.sh
./start.sh
```
