package store

import (
	"fmt"
	"sync"
	"time"
)

type File struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	MimeType   string            `json:"mime_type"`
	Size       int64             `json:"size"`
	ParentID   string            `json:"parent_id"`
	OwnerID    string            `json:"owner_id"`
	SharedWith map[string]string `json:"shared_with"` // userID -> "read"|"write"
	CreatedAt  int64             `json:"created_at"`
	UpdatedAt  int64             `json:"updated_at"`
}

type FileVersion struct {
	Version   int    `json:"version"`
	Content   []byte `json:"content"`
	Size      int64  `json:"size"`
	CreatedAt int64  `json:"created_at"`
}

type Folder struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ParentID  string `json:"parent_id"`
	OwnerID   string `json:"owner_id"`
	CreatedAt int64  `json:"created_at"`
}

type MemoryStore struct {
	mu             sync.RWMutex
	files          map[string]*File
	fileData       map[string][]*FileVersion // fileID -> versions
	folders        map[string]*Folder
	folderChildren map[string][]string // folderID -> child file/folder IDs
	seq            int64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		files:          make(map[string]*File),
		fileData:       make(map[string][]*FileVersion),
		folders:        make(map[string]*Folder),
		folderChildren: make(map[string][]string),
	}
}

func (s *MemoryStore) CreateFile(name, mimeType, parentID, ownerID string, content []byte) *File {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	id := fmt.Sprintf("file_%d", s.seq)
	now := time.Now().UnixMilli()

	f := &File{
		ID:         id,
		Name:       name,
		MimeType:   mimeType,
		Size:       int64(len(content)),
		ParentID:   parentID,
		OwnerID:    ownerID,
		SharedWith: make(map[string]string),
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	s.files[id] = f
	s.fileData[id] = []*FileVersion{{
		Version:   1,
		Content:   content,
		Size:      int64(len(content)),
		CreatedAt: now,
	}}

	if parentID != "" {
		s.folderChildren[parentID] = append(s.folderChildren[parentID], id)
	}

	return f
}

func (s *MemoryStore) UpdateFile(id string, content []byte) (*File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, ok := s.files[id]
	if !ok {
		return nil, fmt.Errorf("file not found")
	}

	versions := s.fileData[id]
	newVersion := len(versions) + 1
	s.fileData[id] = append(versions, &FileVersion{
		Version:   newVersion,
		Content:   content,
		Size:      int64(len(content)),
		CreatedAt: time.Now().UnixMilli(),
	})

	f.Size = int64(len(content))
	f.UpdatedAt = time.Now().UnixMilli()
	return f, nil
}

func (s *MemoryStore) GetFile(id string) *File {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.files[id]
}

func (s *MemoryStore) GetFileContent(id string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	versions, ok := s.fileData[id]
	if !ok || len(versions) == 0 {
		return nil, fmt.Errorf("file not found")
	}
	return versions[len(versions)-1].Content, nil
}

func (s *MemoryStore) GetFileVersions(id string) []*FileVersion {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fileData[id]
}

func (s *MemoryStore) DeleteFile(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.files[id]; !ok {
		return fmt.Errorf("file not found")
	}
	delete(s.files, id)
	delete(s.fileData, id)
	return nil
}

func (s *MemoryStore) CreateFolder(name, parentID, ownerID string) *Folder {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	id := fmt.Sprintf("folder_%d", s.seq)
	f := &Folder{
		ID:        id,
		Name:      name,
		ParentID:  parentID,
		OwnerID:   ownerID,
		CreatedAt: time.Now().UnixMilli(),
	}
	s.folders[id] = f
	if parentID != "" {
		s.folderChildren[parentID] = append(s.folderChildren[parentID], id)
	}
	return f
}

func (s *MemoryStore) GetFolderChildren(folderID string) []interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var children []interface{}
	for _, childID := range s.folderChildren[folderID] {
		if f, ok := s.files[childID]; ok {
			children = append(children, f)
		} else if f, ok := s.folders[childID]; ok {
			children = append(children, f)
		}
	}
	return children
}

func (s *MemoryStore) ShareFile(fileID, userID, permission string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, ok := s.files[fileID]
	if !ok {
		return fmt.Errorf("file not found")
	}
	f.SharedWith[userID] = permission
	return nil
}
