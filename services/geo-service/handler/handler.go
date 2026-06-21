// Package handler provides HTTP handlers and the in-memory HubStore
// for the geo-serviceability service.
package handler

import (
	"sync"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/geo-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/geo-service/service"
	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// HubStore — in-memory implementation of model.HubRepository
// ---------------------------------------------------------------------------

// HubStore is a concurrency-safe in-memory hub repository.
type HubStore struct {
	mu   sync.RWMutex
	hubs map[string]*model.Hub
	list []*model.Hub
}

// NewHubStore creates a new empty HubStore.
func NewHubStore() *HubStore {
	return &HubStore{
		hubs: make(map[string]*model.Hub),
	}
}

// Add stores a hub. Returns nil error for interface compliance.
func (s *HubStore) Add(hub *model.Hub) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hubs[hub.ID] = hub
	s.list = append(s.list, hub)
	return nil
}

// GetAll returns a copy of all hubs in insertion order.
func (s *HubStore) GetAll() []*model.Hub {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.Hub, len(s.list))
	copy(out, s.list)
	return out
}

// GetByID returns a hub by ID.
func (s *HubStore) GetByID(id string) (*model.Hub, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.hubs[id]
	return h, ok
}

// Active returns all hubs with Active==true.
func (s *HubStore) Active() []*model.Hub {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.Hub, 0, len(s.list))
	for _, h := range s.list {
		if h.Active {
			out = append(out, h)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// HTTP Handler
// ---------------------------------------------------------------------------

// Handler wires HTTP endpoints to the geo-service.
type Handler struct {
	svc  *service.Service
	hubs *HubStore
}

// New creates a new Handler.
func New(svc *service.Service, hubs *HubStore) *Handler {
	return &Handler{svc: svc, hubs: hubs}
}

// Register mounts all routes on the provided router group.
func (h *Handler) Register(r *gin.RouterGroup) {
	r.GET("/serviceability", h.CheckServiceability)

	admin := r.Group("/admin")
	admin.POST("/hubs", h.RegisterHub)
	admin.GET("/hubs", h.ListHubs)
}

// CheckServiceability handles GET /serviceability?lat=...&lng=...
func (h *Handler) CheckServiceability(c *gin.Context) {
	var req model.ServiceabilityRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		kit.BadRequest(c, "invalid parameters: lat and lng are required")
		return
	}

	resp, err := h.svc.CheckServiceability(req.Lat, req.Lng)
	if err != nil {
		kit.InternalError(c, err.Error())
		return
	}

	kit.OK(c, resp)
}

// RegisterHub handles POST /admin/hubs with a JSON hub body.
func (h *Handler) RegisterHub(c *gin.Context) {
	var hub model.Hub
	if err := c.ShouldBindJSON(&hub); err != nil {
		kit.BadRequest(c, "invalid hub data: "+err.Error())
		return
	}

	if hub.ID == "" {
		kit.BadRequest(c, "hub ID is required")
		return
	}

	if err := h.hubs.Add(&hub); err != nil {
		kit.InternalError(c, err.Error())
		return
	}

	kit.Created(c, hub)
}

// ListHubs handles GET /admin/hubs.
func (h *Handler) ListHubs(c *gin.Context) {
	kit.OK(c, h.hubs.GetAll())
}
