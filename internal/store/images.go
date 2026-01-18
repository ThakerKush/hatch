package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Image struct {
	ID         string    `json:"id"`
	KernelPath string    `json:"kernel_path"`
	RootfsPath string    `json:"rootfs_path"`
	BootArgs   string    `json:"boot_args"`
	CreatedAt  time.Time `json:"created_at"`
}

type ImageStore struct {
	path   string
	mu     sync.RWMutex
	images map[string]Image
}

func ImagesPath(dataDir string) string {
	return filepath.Join(dataDir, "images.json")
}

func LoadImages(path string) (*ImageStore, error) {
	store := &ImageStore{
		path:   path,
		images: make(map[string]Image),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store, nil
		}
		return nil, err
	}

	if len(data) == 0 {
		return store, nil
	}

	if err := json.Unmarshal(data, &store.images); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *ImageStore) Add(image Image) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.images[image.ID] = image
	return s.save()
}

func (s *ImageStore) Get(id string) (Image, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	image, ok := s.images[id]
	return image, ok
}

func (s *ImageStore) List() []Image {
	s.mu.RLock()
	defer s.mu.RUnlock()
	images := make([]Image, 0, len(s.images))
	for _, image := range s.images {
		images = append(images, image)
	}
	return images
}

func (s *ImageStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.images[id]; !ok {
		return false
	}
	delete(s.images, id)
	_ = s.save()
	return true
}

func (s *ImageStore) save() error {
	data, err := json.MarshalIndent(s.images, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
