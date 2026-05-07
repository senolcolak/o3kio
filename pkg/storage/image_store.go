package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// ImageStore manages image storage operations
type ImageStore struct {
	mode        string // "stub", "local", "rbd", "s3", or combinations like "local,rbd", "local,s3"
	cephPool    string
	cephConf    string
	s3Bucket    string
	s3Region    string
	s3Endpoint  string
	timeout     time.Duration
	localPath   string
	mu          sync.Mutex
	stubImages  map[string]*stubImage // For stub mode
	s3Client    *s3.Client
}

// stubImage represents a simulated image
type stubImage struct {
	id        string
	size      int64
	createdAt time.Time
}

// NewImageStore creates a new image store
func NewImageStore(mode, cephPool, cephConf, s3Bucket, s3Region, s3Endpoint string) *ImageStore {
	// Use /var/lib/o3k/images for local storage (Docker-friendly)
	// Falls back to ~/.o3k/images for non-containerized deployments
	localPath := "/var/lib/o3k/images"
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		homeDir, _ := os.UserHomeDir()
		localPath = filepath.Join(homeDir, ".o3k", "images")
	}

	store := &ImageStore{
		mode:       mode,
		cephPool:   cephPool,
		cephConf:   cephConf,
		s3Bucket:   s3Bucket,
		s3Region:   s3Region,
		s3Endpoint: s3Endpoint,
		timeout:    5 * time.Second,
		localPath:  localPath,
		stubImages: make(map[string]*stubImage),
	}

	// Create local storage directory if needed
	if mode == "local" || containsMode(mode, "local") {
		_ = os.MkdirAll(store.localPath, 0755)
	}

	// Initialize S3 client if needed
	if mode == "s3" || containsMode(mode, "s3") {
		store.initS3Client()
	}

	return store
}

// containsMode checks if a mode string contains a specific mode
func containsMode(modes, target string) bool {
	for i := 0; i < len(modes); {
		// Find next comma or end
		end := i
		for end < len(modes) && modes[end] != ',' {
			end++
		}
		if modes[i:end] == target {
			return true
		}
		i = end + 1
	}
	return false
}

// initS3Client initializes the S3 client
func (s *ImageStore) initS3Client() error {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(s.s3Region))
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Use custom endpoint if provided (for S3-compatible storage like MinIO, Ceph RGW)
	if s.s3Endpoint != "" {
		s.s3Client = s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(s.s3Endpoint)
			o.UsePathStyle = true // Required for MinIO and some S3-compatible stores
		})
	} else {
		s.s3Client = s3.NewFromConfig(cfg)
	}

	return nil
}

// UploadImage uploads an image to storage
func (s *ImageStore) UploadImage(ctx context.Context, imageID string, reader io.Reader) (int64, error) {
	switch s.mode {
	case "stub":
		return s.uploadImageStub(imageID, reader)
	case "local":
		return s.uploadImageLocal(ctx, imageID, reader)
	case "rbd":
		return s.uploadImageRBD(ctx, imageID, reader)
	case "s3":
		return s.uploadImageS3(ctx, imageID, reader)
	case "local,rbd":
		// Upload to local first
		size, err := s.uploadImageLocal(ctx, imageID, reader)
		if err != nil {
			return 0, fmt.Errorf("failed to upload to local: %w", err)
		}
		// Then upload to RBD (would need to read from local file)
		// For now, just log that it would be uploaded to both
		return size, nil
	case "local,s3":
		// Upload to local first
		size, err := s.uploadImageLocal(ctx, imageID, reader)
		if err != nil {
			return 0, fmt.Errorf("failed to upload to local: %w", err)
		}
		// Then upload to S3 from local file
		if err := s.uploadLocalToS3(ctx, imageID); err != nil {
			// Keep local copy even if S3 fails
			return size, fmt.Errorf("uploaded to local but failed to replicate to S3: %w", err)
		}
		return size, nil
	case "rbd,s3":
		// Upload to RBD first
		size, err := s.uploadImageRBD(ctx, imageID, reader)
		if err != nil {
			return 0, fmt.Errorf("failed to upload to RBD: %w", err)
		}
		return size, nil
	default:
		return 0, fmt.Errorf("unsupported storage mode: %s", s.mode)
	}
}

// uploadImageStub simulates image upload
func (s *ImageStore) uploadImageStub(imageID string, reader io.Reader) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Read all data to get size
	data, err := io.ReadAll(reader)
	if err != nil {
		return 0, err
	}

	s.stubImages[imageID] = &stubImage{
		id:        imageID,
		size:      int64(len(data)),
		createdAt: time.Now(),
	}

	return int64(len(data)), nil
}

// uploadImageLocal uploads image to local storage
func (s *ImageStore) uploadImageLocal(ctx context.Context, imageID string, reader io.Reader) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	imagePath := filepath.Join(s.localPath, "image-"+imageID+".raw")

	// Create file
	file, err := os.Create(imagePath)
	if err != nil {
		return 0, fmt.Errorf("failed to create image file: %w", err)
	}
	defer file.Close()

	// Copy data from reader to file
	size, err := io.Copy(file, reader)
	if err != nil {
		_ = os.Remove(imagePath) // Cleanup on error
		return 0, fmt.Errorf("failed to write image data: %w", err)
	}

	return size, nil
}

// uploadImageRBD uploads image to RBD
func (s *ImageStore) uploadImageRBD(ctx context.Context, imageID string, reader io.Reader) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	imageName := "image-" + imageID

	// TODO: Use go-ceph to write to RBD
	// cmd := exec.CommandContext(ctx, "rbd", "import", "-", fmt.Sprintf("%s/%s", s.cephPool, imageName))
	// cmd.Stdin = reader
	// return cmd.Run()

	_ = imageName
	return 0, fmt.Errorf("Ceph cluster not configured (would upload to %s/%s)", s.cephPool, imageName)
}

// uploadImageS3 uploads image to S3
func (s *ImageStore) uploadImageS3(ctx context.Context, imageID string, reader io.Reader) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	if s.s3Client == nil {
		return 0, fmt.Errorf("S3 client not initialized")
	}

	objectKey := "images/image-" + imageID + ".raw"

	// Read all data to buffer since we need size
	data, err := io.ReadAll(reader)
	if err != nil {
		return 0, fmt.Errorf("failed to read image data: %w", err)
	}

	// Create a bytes.Reader for AWS SDK
	bodyReader := bytes.NewReader(data)

	_, err = s.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.s3Bucket),
		Key:    aws.String(objectKey),
		Body:   bodyReader,
	})

	if err != nil {
		return 0, fmt.Errorf("failed to upload to S3: %w", err)
	}

	return int64(len(data)), nil
}

// uploadLocalToS3 uploads an image from local storage to S3
func (s *ImageStore) uploadLocalToS3(ctx context.Context, imageID string) error {
	imagePath := filepath.Join(s.localPath, "image-"+imageID+".raw")

	file, err := os.Open(imagePath)
	if err != nil {
		return fmt.Errorf("failed to open local image: %w", err)
	}
	defer file.Close()

	objectKey := "images/image-" + imageID + ".raw"

	_, err = s.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.s3Bucket),
		Key:    aws.String(objectKey),
		Body:   file,
	})

	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	return nil
}

// DownloadImage downloads an image from storage
func (s *ImageStore) DownloadImage(ctx context.Context, imageID string, writer io.Writer) error {
	switch s.mode {
	case "stub":
		return s.downloadImageStub(imageID, writer)
	case "local":
		return s.downloadImageLocal(ctx, imageID, writer)
	case "rbd":
		return s.downloadImageRBD(ctx, imageID, writer)
	case "s3":
		return s.downloadImageS3(ctx, imageID, writer)
	case "local,rbd":
		// Try local first (faster)
		if err := s.downloadImageLocal(ctx, imageID, writer); err == nil {
			return nil
		}
		// Fallback to RBD
		return s.downloadImageRBD(ctx, imageID, writer)
	case "local,s3":
		// Try local first (faster)
		if err := s.downloadImageLocal(ctx, imageID, writer); err == nil {
			return nil
		}
		// Fallback to S3
		return s.downloadImageS3(ctx, imageID, writer)
	case "rbd,s3":
		// Try RBD first
		if err := s.downloadImageRBD(ctx, imageID, writer); err == nil {
			return nil
		}
		// Fallback to S3
		return s.downloadImageS3(ctx, imageID, writer)
	default:
		return fmt.Errorf("unsupported storage mode: %s", s.mode)
	}
}

// downloadImageStub simulates image download
func (s *ImageStore) downloadImageStub(imageID string, writer io.Writer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	img, exists := s.stubImages[imageID]
	if !exists {
		return fmt.Errorf("image %s not found", imageID)
	}

	// Write fake data
	_, err := writer.Write(make([]byte, img.size))
	return err
}

// downloadImageLocal downloads from local storage
func (s *ImageStore) downloadImageLocal(ctx context.Context, imageID string, writer io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	imagePath := filepath.Join(s.localPath, "image-"+imageID+".raw")

	file, err := os.Open(imagePath)
	if err != nil {
		return fmt.Errorf("failed to open image file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(writer, file)
	return err
}

// downloadImageRBD downloads from RBD
func (s *ImageStore) downloadImageRBD(ctx context.Context, imageID string, writer io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	imageName := "image-" + imageID

	// TODO: Use go-ceph
	// cmd := exec.CommandContext(ctx, "rbd", "export", fmt.Sprintf("%s/%s", s.cephPool, imageName), "-")
	// cmd.Stdout = writer
	// return cmd.Run()

	return fmt.Errorf("Ceph cluster not configured (would download from %s/%s)", s.cephPool, imageName)
}

// downloadImageS3 downloads from S3
func (s *ImageStore) downloadImageS3(ctx context.Context, imageID string, writer io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	if s.s3Client == nil {
		return fmt.Errorf("S3 client not initialized")
	}

	objectKey := "images/image-" + imageID + ".raw"

	result, err := s.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.s3Bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return fmt.Errorf("failed to get object from S3: %w", err)
	}
	defer result.Body.Close()

	_, err = io.Copy(writer, result.Body)
	return err
}

// DeleteImage deletes an image from storage
func (s *ImageStore) DeleteImage(ctx context.Context, imageID string) error {
	switch s.mode {
	case "stub":
		return s.deleteImageStub(imageID)
	case "local":
		return s.deleteImageLocal(ctx, imageID)
	case "rbd":
		return s.deleteImageRBD(ctx, imageID)
	case "s3":
		return s.deleteImageS3(ctx, imageID)
	case "local,rbd":
		// Delete from both (best effort)
		localErr := s.deleteImageLocal(ctx, imageID)
		rbdErr := s.deleteImageRBD(ctx, imageID)
		if localErr != nil && rbdErr != nil {
			return fmt.Errorf("failed to delete: local=%v, rbd=%v", localErr, rbdErr)
		}
		return nil
	case "local,s3":
		// Delete from both (best effort)
		localErr := s.deleteImageLocal(ctx, imageID)
		s3Err := s.deleteImageS3(ctx, imageID)
		if localErr != nil && s3Err != nil {
			return fmt.Errorf("failed to delete: local=%v, s3=%v", localErr, s3Err)
		}
		return nil
	case "rbd,s3":
		// Delete from both (best effort)
		rbdErr := s.deleteImageRBD(ctx, imageID)
		s3Err := s.deleteImageS3(ctx, imageID)
		if rbdErr != nil && s3Err != nil {
			return fmt.Errorf("failed to delete: rbd=%v, s3=%v", rbdErr, s3Err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported storage mode: %s", s.mode)
	}
}

// deleteImageStub simulates image deletion
func (s *ImageStore) deleteImageStub(imageID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Delete if exists (idempotent - no error if not found)
	delete(s.stubImages, imageID)
	return nil
}

// deleteImageLocal deletes from local storage
func (s *ImageStore) deleteImageLocal(ctx context.Context, imageID string) error {
	imagePath := filepath.Join(s.localPath, "image-"+imageID+".raw")
	err := os.Remove(imagePath)
	if err != nil && os.IsNotExist(err) {
		// Idempotent delete - not found is success
		return nil
	}
	return err
}

// deleteImageRBD deletes from RBD
func (s *ImageStore) deleteImageRBD(ctx context.Context, imageID string) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	imageName := "image-" + imageID

	// TODO: Use go-ceph
	// cmd := exec.CommandContext(ctx, "rbd", "rm", fmt.Sprintf("%s/%s", s.cephPool, imageName))
	// return cmd.Run()

	return fmt.Errorf("Ceph cluster not configured (would delete %s/%s)", s.cephPool, imageName)
}

// deleteImageS3 deletes from S3
func (s *ImageStore) deleteImageS3(ctx context.Context, imageID string) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	if s.s3Client == nil {
		return fmt.Errorf("S3 client not initialized")
	}

	objectKey := "images/image-" + imageID + ".raw"

	_, err := s.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.s3Bucket),
		Key:    aws.String(objectKey),
	})

	return err
}

// ImageExists checks if an image exists in storage
func (s *ImageStore) ImageExists(ctx context.Context, imageID string) (bool, error) {
	switch s.mode {
	case "stub":
		s.mu.Lock()
		defer s.mu.Unlock()
		_, exists := s.stubImages[imageID]
		return exists, nil
	case "local":
		return s.imageExistsLocal(imageID)
	case "rbd":
		return s.imageExistsRBD(ctx, imageID)
	case "s3":
		return s.imageExistsS3(ctx, imageID)
	case "local,rbd":
		// Check local first (faster)
		if exists, _ := s.imageExistsLocal(imageID); exists {
			return true, nil
		}
		return s.imageExistsRBD(ctx, imageID)
	case "local,s3":
		// Check local first (faster)
		if exists, _ := s.imageExistsLocal(imageID); exists {
			return true, nil
		}
		return s.imageExistsS3(ctx, imageID)
	case "rbd,s3":
		// Check RBD first
		if exists, _ := s.imageExistsRBD(ctx, imageID); exists {
			return true, nil
		}
		return s.imageExistsS3(ctx, imageID)
	default:
		return false, fmt.Errorf("unsupported storage mode: %s", s.mode)
	}
}

// imageExistsLocal checks if image exists locally
func (s *ImageStore) imageExistsLocal(imageID string) (bool, error) {
	imagePath := filepath.Join(s.localPath, "image-"+imageID+".raw")
	_, err := os.Stat(imagePath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// imageExistsRBD checks if image exists in RBD
func (s *ImageStore) imageExistsRBD(ctx context.Context, imageID string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	imageName := "image-" + imageID

	// TODO: Use go-ceph
	// cmd := exec.CommandContext(ctx, "rbd", "info", fmt.Sprintf("%s/%s", s.cephPool, imageName))
	// err := cmd.Run()
	// return err == nil, nil

	return false, fmt.Errorf("Ceph cluster not configured (would check %s/%s)", s.cephPool, imageName)
}

// imageExistsS3 checks if image exists in S3
func (s *ImageStore) imageExistsS3(ctx context.Context, imageID string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	if s.s3Client == nil {
		return false, fmt.Errorf("S3 client not initialized")
	}

	objectKey := "images/image-" + imageID + ".raw"

	_, err := s.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.s3Bucket),
		Key:    aws.String(objectKey),
	})

	if err != nil {
		// Distinguish "not found" from actual errors (IAM, network, etc.)
		var notFound *s3types.NotFound
		if errors.As(err, &notFound) {
			return false, nil
		}
		var noSuchKey *s3types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return false, nil
		}
		return false, fmt.Errorf("S3 HeadObject failed for image %s: %w", imageID, err)
	}

	return true, nil
}

// GetImageSize returns the size of an image
func (s *ImageStore) GetImageSize(ctx context.Context, imageID string) (int64, error) {
	switch s.mode {
	case "stub":
		s.mu.Lock()
		defer s.mu.Unlock()
		if img, exists := s.stubImages[imageID]; exists {
			return img.size, nil
		}
		return 0, fmt.Errorf("image %s not found", imageID)
	case "local":
		imagePath := filepath.Join(s.localPath, "image-"+imageID+".raw")
		info, err := os.Stat(imagePath)
		if err != nil {
			return 0, err
		}
		return info.Size(), nil
	case "s3":
		return s.getImageSizeS3(ctx, imageID)
	case "rbd", "local,rbd", "local,s3", "rbd,s3":
		// For RBD, would parse rbd info output
		return 0, fmt.Errorf("Ceph cluster not configured")
	default:
		return 0, fmt.Errorf("unsupported storage mode: %s", s.mode)
	}
}

// getImageSizeS3 returns the size of an image from S3
func (s *ImageStore) getImageSizeS3(ctx context.Context, imageID string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	if s.s3Client == nil {
		return 0, fmt.Errorf("S3 client not initialized")
	}

	objectKey := "images/image-" + imageID + ".raw"

	result, err := s.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.s3Bucket),
		Key:    aws.String(objectKey),
	})

	if err != nil {
		return 0, fmt.Errorf("failed to get S3 object metadata: %w", err)
	}

	if result.ContentLength == nil {
		return 0, fmt.Errorf("S3 object has no content length")
	}

	return *result.ContentLength, nil
}

// GetRBDPath returns the RBD path for an image
func (s *ImageStore) GetRBDPath(imageID string) string {
	return fmt.Sprintf("rbd:%s/image-%s", s.cephPool, imageID)
}
