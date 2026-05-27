//go:build !ceph
// +build !ceph

package storage

import (
	"context"
	"fmt"
	"io"
)

// rbdNotCompiledMsg is the stub error returned when the binary was built
// without `-tags ceph`. We render the pool/image name in the message so
// operators can still see what would have happened, matching the behaviour
// of the previous shell-out stubs.
func (s *ImageStore) rbdNotCompiledMsg(action, imageID string) string {
	return fmt.Sprintf("Ceph RBD support not compiled (would %s %s/image-%s; rebuild with -tags ceph)",
		action, s.cephPool, imageID)
}

// initCephConnection is a no-op when built without the ceph tag.
func (s *ImageStore) initCephConnection() error {
	return fmt.Errorf("Ceph RBD support not compiled (build with -tags ceph)")
}

// closeCephConnection is a no-op stub.
func (s *ImageStore) closeCephConnection() error {
	return nil
}

func (s *ImageStore) uploadImageRBD(ctx context.Context, imageID string, reader io.Reader) (int64, error) {
	return 0, fmt.Errorf("%s", s.rbdNotCompiledMsg("upload to", imageID))
}

func (s *ImageStore) downloadImageRBD(ctx context.Context, imageID string, writer io.Writer) error {
	return fmt.Errorf("%s", s.rbdNotCompiledMsg("download from", imageID))
}

func (s *ImageStore) deleteImageRBD(ctx context.Context, imageID string) error {
	return fmt.Errorf("%s", s.rbdNotCompiledMsg("delete", imageID))
}

func (s *ImageStore) imageExistsRBD(ctx context.Context, imageID string) (bool, error) {
	return false, fmt.Errorf("%s", s.rbdNotCompiledMsg("check", imageID))
}

func (s *ImageStore) getImageSizeRBD(ctx context.Context, imageID string) (int64, error) {
	return 0, fmt.Errorf("%s", s.rbdNotCompiledMsg("size", imageID))
}
