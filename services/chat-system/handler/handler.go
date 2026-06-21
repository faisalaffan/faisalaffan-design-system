package handler

import (
	"github.com/faisalaffan/faisalaffan-design-system/services/chat-system/room"
	"github.com/faisalaffan/faisalaffan-design-system/services/chat-system/ws"
	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/gin-gonic/gin"
)

type ChatHandler struct {
	mgr *room.Manager
	wsh *ws.Handler
}

func NewChatHandler(mgr *room.Manager) *ChatHandler {
	return &ChatHandler{mgr: mgr, wsh: ws.NewHandler(mgr)}
}

func (h *ChatHandler) WebSocket(c *gin.Context) {
	h.wsh.ServeWS(c.Writer, c.Request)
}

func (h *ChatHandler) Messages(c *gin.Context) {
	roomName := c.Param("room")
	rm, err := h.mgr.Get(roomName)
	if err != nil {
		kit.NotFound(c, "room not found")
		return
	}
	kit.OK(c, gin.H{"room": roomName, "messages": rm.History()})
}

func (h *ChatHandler) ListRooms(c *gin.Context) {
	rooms := h.mgr.List()
	kit.OK(c, gin.H{"rooms": rooms})
}

func (h *ChatHandler) Register(r *gin.RouterGroup) {
	r.GET("/ws", h.WebSocket)
	r.GET("/api/rooms", h.ListRooms)
	r.GET("/api/rooms/:room/messages", h.Messages)
}
