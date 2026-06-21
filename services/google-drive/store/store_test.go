package store

import "testing"

func TestMemoryStore_CreateFile(t *testing.T) {
	s := NewMemoryStore()
	f := s.CreateFile("doc.txt", "text/plain", "", "alice", []byte("hello"))
	if f.ID == "" {
		t.Error("expected non-empty ID")
	}
	if f.Name != "doc.txt" {
		t.Errorf("expected doc.txt, got %s", f.Name)
	}
	if f.Size != 5 {
		t.Errorf("expected size 5, got %d", f.Size)
	}
}

func TestMemoryStore_UpdateFile_Versioning(t *testing.T) {
	s := NewMemoryStore()
	f := s.CreateFile("v.txt", "text/plain", "", "a", []byte("v1"))
	s.UpdateFile(f.ID, []byte("v2xxx"))

	versions := s.GetFileVersions(f.ID)
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	if versions[1].Version != 2 {
		t.Errorf("expected version 2, got %d", versions[1].Version)
	}
	if string(versions[1].Content) != "v2xxx" {
		t.Errorf("expected v2xxx, got %s", string(versions[1].Content))
	}
}

func TestMemoryStore_CreateFolder(t *testing.T) {
	s := NewMemoryStore()
	f := s.CreateFolder("docs", "", "bob")
	if f.Name != "docs" {
		t.Errorf("expected docs, got %s", f.Name)
	}
}

func TestMemoryStore_ShareFile(t *testing.T) {
	s := NewMemoryStore()
	f := s.CreateFile("s.txt", "text/plain", "", "a", []byte("x"))
	s.ShareFile(f.ID, "b", "read")

	got := s.GetFile(f.ID)
	if got.SharedWith["b"] != "read" {
		t.Errorf("expected read permission for b, got %s", got.SharedWith["b"])
	}
}

func TestMemoryStore_DeleteFile(t *testing.T) {
	s := NewMemoryStore()
	f := s.CreateFile("d.txt", "text/plain", "", "a", []byte("x"))
	s.DeleteFile(f.ID)

	if s.GetFile(f.ID) != nil {
		t.Error("expected nil after delete")
	}
}
