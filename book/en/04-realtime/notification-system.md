# Notification System

Pub/sub notification delivery across multiple channels with a `Sender` interface. In-app notifications are persisted; email and push are simulated via log output.

Port **8083** | Package `notification-system/`

---

## Architecture

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

All senders implement a common interface, allowing new channels (SMS, Slack, webhook) to be added without changing the dispatch logic.

```go
type Sender interface {
    Send(n *Notification) error
    Name() Type
}
```

---

## Code Example

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

### In-App Sender Implementation

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

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/send` | Send a notification (body: user_id, channel, title, body) |
| `GET` | `/notifications?user=X` | List notifications for a user |
| `GET` | `/notifications/:id` | Get a single notification by ID |

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

## Technical Decisions

### Interface-Based Dispatch

The `Sender` interface decouples notification routing from delivery logic. Adding a new channel (SMS, Slack, webhook) requires only:

1. Implement the `Sender` interface.
2. Register the sender in `Service.New()`.

No changes to the handler or service dispatch logic.

### Three Delivery Semantics

| Channel | Behaviour |
|---------|-----------|
| `in_app` | Persisted to in-memory store, status becomes `"delivered"`, retrievable via GET endpoints |
| `email` | Simulated via `log.Printf`, status becomes `"sent"`, no actual SMTP call |
| `push` | Simulated via `log.Printf`, status becomes `"sent"`, no actual FCM/APNs call |

### In-Memory Store

Notifications are stored in a flat slice on `channel.Store`. Querying by user ID does a linear scan -- acceptable for the demo scale but would need indexing in production (e.g., per-user lists or database queries).

---

## Key Files

| File | Purpose |
|------|---------|
| `channel/channel.go` | Sender interface, Notification model, transport implementations |
| `service/service.go` | Business logic: channel routing, ID generation |
| `handler/handler.go` | HTTP handlers for send and retrieval |

## Source Code

[View on GitHub](https://github.com/faisalaffan/faisalaffan-design-system/blob/dev/services/notification-system/main.go)
