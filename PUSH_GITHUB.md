# GitHub'a Push Etme

Proje hazır. Aşağıdaki adımlardan **birini** tamamla:

---

## Yöntem 1: GitHub CLI ile (Önerilen)

### 1. GitHub'a giriş yap

Terminal'de çalıştır (tarayıcı açılacak, oturum açman gerekecek):

```bash
gh auth login
```

- Host: GitHub.com
- Protocol: HTTPS
- Authenticate: Login with a web browser

### 2. Private repo oluştur ve push et

```bash
cd "/Users/mete/Desktop/projects/Eldorado Boyt"
gh repo create eldorado-bot --private --source=. --push
```

---

## Yöntem 2: Manuel

### 1. GitHub'da repo oluştur

1. https://github.com/new adresine git
2. Repository name: `eldorado-bot`
3. **Private** seç
4. "Add a README file" işaretli **olmasın**
5. Create repository

### 2. Remote ekle ve push et

```bash
cd "/Users/mete/Desktop/projects/Eldorado Boyt"
git remote add origin https://github.com/KULLANICI_ADINI_YAZ/eldorado-bot.git
git push -u origin main
```

`KULLANICI_ADINI_YAZ` yerine GitHub kullanıcı adını yaz.
