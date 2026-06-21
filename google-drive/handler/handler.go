package handler

import (
	"io"

	"github.com/faisalaffan/faisalaffan-design-system/google-drive/store"
	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/gin-gonic/gin"
)

type DriveHandler struct {
	store *store.MemoryStore
}

func NewDriveHandler(s *store.MemoryStore) *DriveHandler {
	return &DriveHandler{store: s}
}

func (h *DriveHandler) CreateFile(c *gin.Context) {
	name := c.Query("name")
	ownerID := c.Query("owner_id")
	parentID := c.Query("parent_id")
	mimeType := c.DefaultQuery("mime_type", "application/octet-stream")

	if name == "" || ownerID == "" {
		kit.BadRequest(c, "name and owner_id are required")
		return
	}

	content, _ := io.ReadAll(c.Request.Body)
	if len(content) == 0 {
		// Try multipart file
		file, err := c.FormFile("file")
		if err != nil {
			kit.BadRequest(c, "file content is required (use multipart file upload or raw body)")
			return
		}
		f, _ := file.Open()
		content, _ = io.ReadAll(f)
		f.Close()
	}

	f := h.store.CreateFile(name, mimeType, parentID, ownerID, content)
	kit.Created(c, gin.H{"file": f})
}

func (h *DriveHandler) GetFile(c *gin.Context) {
	id := c.Param("id")
	f := h.store.GetFile(id)
	if f == nil {
		kit.NotFound(c, "file not found")
		return
	}
	content, _ := h.store.GetFileContent(id)
	kit.OK(c, gin.H{"file": f, "content_size": len(content)})
}

func (h *DriveHandler) DownloadFile(c *gin.Context) {
	id := c.Param("id")
	f := h.store.GetFile(id)
	if f == nil {
		kit.NotFound(c, "file not found")
		return
	}
	content, err := h.store.GetFileContent(id)
	if err != nil {
		kit.NotFound(c, "file content not found")
		return
	}
	c.Data(200, f.MimeType, content)
}

func (h *DriveHandler) UpdateFile(c *gin.Context) {
	id := c.Param("id")
	content, _ := io.ReadAll(c.Request.Body)
	if len(content) == 0 {
		kit.BadRequest(c, "content is required")
		return
	}
	f, err := h.store.UpdateFile(id, content)
	if err != nil {
		kit.NotFound(c, "file not found")
		return
	}
	kit.OK(c, gin.H{"file": f})
}

func (h *DriveHandler) DeleteFile(c *gin.Context) {
	id := c.Param("id")
	if err := h.store.DeleteFile(id); err != nil {
		kit.NotFound(c, "file not found")
		return
	}
	kit.OK(c, gin.H{"deleted": true})
}

func (h *DriveHandler) GetVersions(c *gin.Context) {
	id := c.Param("id")
	versions := h.store.GetFileVersions(id)
	if versions == nil {
		kit.NotFound(c, "file not found")
		return
	}
	kit.OK(c, gin.H{"versions": versions})
}

type createFolderRequest struct {
	Name     string `json:"name" binding:"required"`
	ParentID string `json:"parent_id"`
	OwnerID  string `json:"owner_id" binding:"required"`
}

func (h *DriveHandler) CreateFolder(c *gin.Context) {
	var req createFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "name and owner_id are required")
		return
	}
	f := h.store.CreateFolder(req.Name, req.ParentID, req.OwnerID)
	kit.Created(c, gin.H{"folder": f})
}

func (h *DriveHandler) GetFolderChildren(c *gin.Context) {
	id := c.Param("id")
	children := h.store.GetFolderChildren(id)
	if children == nil {
		children = make([]interface{}, 0)
	}
	kit.OK(c, gin.H{"children": children})
}

type shareRequest struct {
	UserID     string `json:"user_id" binding:"required"`
	Permission string `json:"permission" binding:"required"`
}

func (h *DriveHandler) ShareFile(c *gin.Context) {
	id := c.Param("id")
	var req shareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "user_id and permission are required")
		return
	}
	if err := h.store.ShareFile(id, req.UserID, req.Permission); err != nil {
		kit.NotFound(c, "file not found")
		return
	}
	kit.OK(c, gin.H{"shared": true})
}

func (h *DriveHandler) Register(r *gin.RouterGroup) {
	r.POST("/files", h.CreateFile)
	r.GET("/files/:id", h.GetFile)
	r.GET("/files/:id/download", h.DownloadFile)
	r.PUT("/files/:id", h.UpdateFile)
	r.DELETE("/files/:id", h.DeleteFile)
	r.GET("/files/:id/versions", h.GetVersions)
	r.POST("/files/:id/share", h.ShareFile)
	r.POST("/folders", h.CreateFolder)
	r.GET("/folders/:id/children", h.GetFolderChildren)
}
