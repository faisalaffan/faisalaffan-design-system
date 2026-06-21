# Notification System

Pengiriman notifikasi pub/sub di berbagai saluran dengan interface `Sender`. Notifikasi in-app dipertahankan; email dan push disimulasikan melalui output log.

Port **8083** | Paket `notification-system/`

---

## Arsitektur

```mermaid
%%{init: {"theme": "base", "themeVariables": {"background": "#ffffff"}}}%%
flowchart LR
    Client -->|POST /send| SendHandler
    SendHandler --> Service
    
    subgraph Service["Notification Service"]
        Service -->|channel: in_app| InAppSender
        Service -->|channel: email| EmailSender
        Service -->|channel: push| PushSender
    end
    
    InAppSender -->|persist| Store[(In-Memory Store)]
    EmailSender -->|log simulate| Stdout
    PushSender -->|log simulate| Stdout
    
    Client -->|GET /notifications?user=X| GetHandler
    GetHandler --> Store
```

### Sender Interface

Semua pengirim mengimplementasikan interface yang sama, memungkinkan saluran baru (SMS, Slack, webhook) ditambahkan tanpa mengubah logika pengiriman.

```go
type Sender interface {
    Send(n *Notification) error
    Name() Type
}
```

---

## Contoh Kode

### Service Layer: Channel Routing

```go
type Service struct {
    senders map[channel.Type]channel.Sender
    store   *channel.Store
}

func New(store *channel.Store) *Service {
    svc := &Service{
        senders: make(map[channel.Type]channel.Sender),
        store:   store,
    }
    svc.senders[channel.InApp] = channel.NewInAppSender(store)
    svc.senders[channel.Email] = &channel.EmailSender{}
    svc.senders[channel.Push]  = &channel.PushSender{}
    return svc
}

func (s *Service) Send(userID string, ch channel.Type, title, body string) (*channel.Notification, error) {
    sender, ok := s.senders[ch]
    if !ok {
        return nil, fmt.Errorf("unknown channel: %s", ch)
    }
    n := &channel.Notification{
        ID:        fmt.Sprintf("notif_%d", time.Now().UnixMilli()),
        UserID:    userID,
        Channel:   ch,
        Title:     title,
        Body:      body,
        CreatedAt: time.Now().UnixMilli(),
    }
    if err := sender.Send(n); err != nil {
        n.Status = "failed"
        return n, err
    }
    return n, nil
}
```

### Implementasi In-App Sender

```go
func (s *InAppSender) Send(n *Notification) error {
    n.Status = "delivered"
    s.store.Save(n)
    return nil
}
```

### Simulated Senders

```go
type EmailSender struct{}

func (s *EmailSender) Send(n *Notification) error {
    log.Printf("[EMAIL] To: %s | Subject: %s | Body: %s", n.UserID, n.Title, n.Body)
    n.Status = "sent"
    return nil
}

type PushSender struct{}

func (s *PushSender) Send(n *Notification) error {
    log.Printf("[PUSH] To: %s | Title: %s | Body: %s", n.UserID, n.Title, n.Body)
    n.Status = "sent"
    return nil
}
```

---

## API Endpoints

| Method | Path | Deskripsi |
|--------|------|-------------|
| `POST` | `/send` | Mengirim notifikasi (body: user_id, channel, title, body) |
| `GET` | `/notifications?user=X` | Mendaftar notifikasi untuk pengguna |
| `GET` | `/notifications/:id` | Mendapatkan satu notifikasi berdasarkan ID |

### POST /send Request Body

```json
{
    "user_id": "user_001",
    "channel": "in_app",
    "title": "New Message",
    "body": "You have a new message from Alice"
}
```

---

## Keputusan Teknis

### Dispatch Berbasis Interface

Interface `Sender` memisahkan perutean notifikasi dari logika pengiriman. Menambahkan saluran baru (SMS, Slack, webhook) hanya membutuhkan:

1. Mengimplementasikan interface `Sender`.
2. Mendaftarkan pengirim di `Service.New()`.

Tidak ada perubahan pada handler atau logika dispatch layanan.

### Tiga Semantik Pengiriman

| Saluran | Perilaku |
|---------|-----------|
| `in_app` | Disimpan ke penyimpanan in-memory, status menjadi `"delivered"`, dapat diambil melalui endpoint GET |
| `email` | Disimulasikan melalui `log.Printf`, status menjadi `"sent"`, tanpa panggilan SMTP aktual |
| `push` | Disimulasikan melalui `log.Printf`, status menjadi `"sent"`, tanpa panggilan FCM/APNs aktual |

### In-Memory Store

Notifikasi disimpan dalam irisan datar di `channel.Store`. Kueri berdasarkan ID pengguna melakukan pemindaian linear -- dapat diterima untuk skala demo tetapi akan membutuhkan pengindeksan di produksi (mis., daftar per-pengguna atau kueri basis data).

---

## File Kunci

| File | Tujuan |
|------|---------|
| `channel/channel.go` | Interface Sender, model Notification, implementasi transport |
| `service/service.go` | Logika bisnis: perutean saluran, pembuatan ID |
| `handler/handler.go` | HTTP handlers untuk pengiriman dan pengambilan |
