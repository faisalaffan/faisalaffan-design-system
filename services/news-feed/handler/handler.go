package handler

import (
	"strconv"

	"github.com/faisalaffan/faisalaffan-design-system/services/news-feed/store"
	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/gin-gonic/gin"
)

type NewsFeedHandler struct {
	store *store.MemoryStore
}

func NewNewsFeedHandler(s *store.MemoryStore) *NewsFeedHandler {
	return &NewsFeedHandler{store: s}
}

type createPostRequest struct {
	UserID  string `json:"user_id" binding:"required"`
	Content string `json:"content" binding:"required"`
}

func (h *NewsFeedHandler) CreatePost(c *gin.Context) {
	var req createPostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "user_id and content are required")
		return
	}
	p := h.store.CreatePost(req.UserID, req.Content)
	kit.Created(c, gin.H{"post": p})
}

type followRequest struct {
	FollowerID string `json:"follower_id" binding:"required"`
	FolloweeID string `json:"followee_id" binding:"required"`
}

func (h *NewsFeedHandler) Follow(c *gin.Context) {
	var req followRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "follower_id and followee_id are required")
		return
	}
	if !h.store.Follow(req.FollowerID, req.FolloweeID) {
		kit.BadRequest(c, "invalid user IDs")
		return
	}
	kit.OK(c, gin.H{"followed": true})
}

func (h *NewsFeedHandler) Timeline(c *gin.Context) {
	userID := c.Query("user")
	if userID == "" {
		kit.BadRequest(c, "user query param required")
		return
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	posts := h.store.GetTimeline(userID, offset, limit)
	if posts == nil {
		posts = []*store.Post{}
	}
	kit.OK(c, gin.H{"timeline": posts})
}

func (h *NewsFeedHandler) UserPosts(c *gin.Context) {
	userID := c.Param("id")
	posts := h.store.GetUserPosts(userID)
	if posts == nil {
		posts = []*store.Post{}
	}
	kit.OK(c, gin.H{"posts": posts})
}

type createUserRequest struct {
	ID string `json:"id" binding:"required"`
}

func (h *NewsFeedHandler) CreateUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "id is required")
		return
	}
	u := h.store.CreateUser(req.ID)
	kit.Created(c, gin.H{"user": u})
}

func (h *NewsFeedHandler) Register(r *gin.RouterGroup) {
	r.POST("/users", h.CreateUser)
	r.POST("/posts", h.CreatePost)
	r.POST("/follow", h.Follow)
	r.GET("/timeline", h.Timeline)
	r.GET("/users/:id/posts", h.UserPosts)
}
