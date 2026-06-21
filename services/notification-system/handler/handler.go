package handler

import (
	"github.com/faisalaffan/faisalaffan-design-system/services/notification-system/channel"
	"github.com/faisalaffan/faisalaffan-design-system/services/notification-system/service"
	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/gin-gonic/gin"
)

type NotificationHandler struct {
	svc *service.Service
}

func NewNotificationHandler(svc *service.Service) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

type sendRequest struct {
	UserID  string `json:"user_id" binding:"required"`
	Channel string `json:"channel" binding:"required"`
	Title   string `json:"title" binding:"required"`
	Body    string `json:"body" binding:"required"`
}

func (h *NotificationHandler) Send(c *gin.Context) {
	var req sendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "user_id, channel, title, and body are required")
		return
	}

	n, err := h.svc.Send(req.UserID, channel.Type(req.Channel), req.Title, req.Body)
	if err != nil {
		kit.BadRequest(c, err.Error())
		return
	}

	kit.Created(c, gin.H{"notification": n})
}

func (h *NotificationHandler) GetByUser(c *gin.Context) {
	userID := c.Query("user")
	if userID == "" {
		kit.BadRequest(c, "user query param is required")
		return
	}

	notifs := h.svc.GetByUser(userID)
	if notifs == nil {
		notifs = make([]*channel.Notification, 0)
	}
	kit.OK(c, gin.H{"notifications": notifs})
}

func (h *NotificationHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	n := h.svc.GetByID(id)
	if n == nil {
		kit.NotFound(c, "notification not found")
		return
	}
	kit.OK(c, gin.H{"notification": n})
}

func (h *NotificationHandler) Register(r *gin.RouterGroup) {
	r.POST("/send", h.Send)
	r.GET("/notifications", h.GetByUser)
	r.GET("/notifications/:id", h.GetByID)
}
