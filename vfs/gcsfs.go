// +build !nogcs

package vfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/eikenb/pipeat"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/drakkan/sftpgo/logger"
	"github.com/drakkan/sftpgo/metrics"
	"github.com/drakkan/sftpgo/version"
)

var (
	// we can use fields selection only when we don't need directory-like results
	// with folders
	gcsDefaultFieldsSelection = []string{"Name", "Size", "Deleted", "Updated"}
)

// GCSFs is a Fs implementation for Google Cloud Storage.
type GCSFs struct {
	connectionID   string
	localTempDir   string
	config         GCSFsConfig
	svc            *storage.Client
	ctxTimeout     time.Duration
	ctxLongTimeout time.Duration
}

func init() {
	version.AddFeature("+gcs")
}

// NewGCSFs returns an GCSFs object that allows to interact with Google Cloud Storage
func NewGCSFs(connectionID, localTempDir string, config GCSFsConfig) (Fs, error) {
	var err error
	fs := GCSFs{
		connectionID:   connectionID,
		localTempDir:   localTempDir,
		config:         config,
		ctxTimeout:     30 * time.Second,
		ctxLongTimeout: 300 * time.Second,
	}
	if err = ValidateGCSFsConfig(&fs.config, fs.config.CredentialFile); err != nil {
		return fs, err
	}
	ctx := context.Background()
	if fs.config.AutomaticCredentials > 0 {
		fs.svc, err = storage.NewClient(ctx)
	} else {
		fs.svc, err = storage.NewClient(ctx, option.WithCredentialsFile(fs.config.CredentialFile))
	}
	return fs, err
}

// Name returns the name for the Fs implementation
func (fs GCSFs) Name() string {
	return fmt.Sprintf("GCSFs bucket: %#v", fs.config.Bucket)
}

// ConnectionID returns the SSH connection ID associated to this Fs implementation
func (fs GCSFs) ConnectionID() string {
	return fs.connectionID
}

// Stat returns a FileInfo describing the named file
func (fs GCSFs) Stat(name string) (os.FileInfo, error) {
	var result FileInfo
	var err error
	if len(name) == 0 || name == "." {
		err := fs.checkIfBucketExists()
		if err != nil {
			return result, err
		}
		return NewFileInfo(name, true, 0, time.Now()), nil
	}
	if fs.config.KeyPrefix == name+"/" {
		return NewFileInfo(name, true, 0, time.Now()), nil
	}
	prefix := fs.getPrefixForStat(name)
	query := &storage.Query{Prefix: prefix, Delimiter: "/"}
	ctx, cancelFn := context.WithDeadline(context.Background(), time.Now().Add(fs.ctxTimeout))
	defer cancelFn()
	bkt := fs.svc.Bucket(fs.config.Bucket)
	it := bkt.Objects(ctx, query)
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			metrics.GCSListObjectsCompleted(err)
			return result, err
		}
		if len(attrs.Prefix) > 0 {
			if fs.isEqual(attrs.Prefix, name) {
				result = NewFileInfo(name, true, 0, time.Now())
				break
			}
		} else {
			if !attrs.Deleted.IsZero() {
				continue
			}
			if fs.isEqual(attrs.Name, name) {
				isDir := strings.HasSuffix(attrs.Name, "/")
				result = NewFileInfo(name, isDir, attrs.Size, attrs.Updated)
				if !isDir {
					result.setContentType(attrs.ContentType)
				}
				break
			}
		}
	}
	metrics.GCSListObjectsCompleted(nil)
	if len(result.Name()) == 0 {
		err = errors.New("404 no such file or directory")
	}
	return result, err
}

// Lstat returns a FileInfo describing the named file
func (fs GCSFs) Lstat(name string) (os.FileInfo, error) {
	return fs.Stat(name)
}

// Open opens the named file for reading
func (fs GCSFs) Open(name string, offset int64) (*os.File, *pipeat.PipeReaderAt, func(), error) {
	r, w, err := pipeat.PipeInDir(fs.localTempDir)
	if err != nil {
		return nil, nil, nil, err
	}
	bkt := fs.svc.Bucket(fs.config.Bucket)
	obj := bkt.Object(name)
	ctx, cancelFn := context.WithCancel(context.Background())
	objectReader, err := obj.NewRangeReader(ctx, offset, -1)
	if err == nil && offset > 0 && objectReader.Attrs.ContentEncoding == "gzip" {
		err = fmt.Errorf("Range request is not possible for gzip content encoding, requested offset %v", offset)
		objectReader.Close()
	}
	if err != nil {
		r.Close()
		w.Close()
		cancelFn()
		return nil, nil, nil, err
	}
	go func() {
		defer cancelFn()
		defer objectReader.Close()
		n, err := io.Copy(w, objectReader)
		w.CloseWithError(err) //nolint:errcheck
		fsLog(fs, logger.LevelDebug, "download completed, path: %#v size: %v, err: %v", name, n, err)
		metrics.GCSTransferCompleted(n, 1, err)
	}()
	return nil, r, cancelFn, nil
}

// Create creates or opens the named file for writing
func (fs GCSFs) Create(name string, flag int) (*os.File, *PipeWriter, func(), error) {
	r, w, err := pipeat.PipeInDir(fs.localTempDir)
	if err != nil {
		return nil, nil, nil, err
	}
	p := NewPipeWriter(w)
	bkt := fs.svc.Bucket(fs.config.Bucket)
	obj := bkt.Object(name)
	ctx, cancelFn := context.WithCancel(context.Background())
	objectWriter := obj.NewWriter(ctx)
	if len(fs.config.StorageClass) > 0 {
		objectWriter.ObjectAttrs.StorageClass = fs.config.StorageClass
	}
	go func() {
		defer cancelFn()
		defer objectWriter.Close()
		n, err := io.Copy(objectWriter, r)
		r.CloseWithError(err) //nolint:errcheck
		p.Done(err)
		fsLog(fs, logger.LevelDebug, "upload completed, path: %#v, readed bytes: %v, err: %v", name, n, err)
		metrics.GCSTransferCompleted(n, 0, err)
	}()
	return nil, p, cancelFn, nil
}

// Rename renames (moves) source to target.
// We don't support renaming non empty directories since we should
// rename all the contents too and this could take long time: think
// about directories with thousands of files, for each file we should
// execute a CopyObject call.
func (fs GCSFs) Rename(source, target string) error {
	if source == target {
		return nil
	}
	fi, err := fs.Stat(source)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		contents, err := fs.ReadDir(source)
		if err != nil {
			return err
		}
		if len(contents) > 0 {
			return fmt.Errorf("Cannot rename non empty directory: %#v", source)
		}
		if !strings.HasSuffix(source, "/") {
			source += "/"
		}
		if !strings.HasSuffix(target, "/") {
			target += "/"
		}
	}
	src := fs.svc.Bucket(fs.config.Bucket).Object(source)
	dst := fs.svc.Bucket(fs.config.Bucket).Object(target)
	ctx, cancelFn := context.WithDeadline(context.Background(), time.Now().Add(fs.ctxTimeout))
	defer cancelFn()
	copier := dst.CopierFrom(src)
	if len(fs.config.StorageClass) > 0 {
		copier.StorageClass = fs.config.StorageClass
	}
	_, err = copier.Run(ctx)
	metrics.GCSCopyObjectCompleted(err)
	if err != nil {
		return err
	}
	return fs.Remove(source, fi.IsDir())
}

// Remove removes the named file or (empty) directory.
func (fs GCSFs) Remove(name string, isDir bool) error {
	if isDir {
		contents, err := fs.ReadDir(name)
		if err != nil {
			return err
		}
		if len(contents) > 0 {
			return fmt.Errorf("Cannot remove non empty directory: %#v", name)
		}
		if !strings.HasSuffix(name, "/") {
			name += "/"
		}
	}
	ctx, cancelFn := context.WithDeadline(context.Background(), time.Now().Add(fs.ctxTimeout))
	defer cancelFn()
	err := fs.svc.Bucket(fs.config.Bucket).Object(name).Delete(ctx)
	metrics.GCSDeleteObjectCompleted(err)
	return err
}

// Mkdir creates a new directory with the specified name and default permissions
func (fs GCSFs) Mkdir(name string) error {
	_, err := fs.Stat(name)
	if !fs.IsNotExist(err) {
		return err
	}
	if !strings.HasSuffix(name, "/") {
		name += "/"
	}
	_, w, _, err := fs.Create(name, 0)
	if err != nil {
		return err
	}
	return w.Close()
}

// Symlink creates source as a symbolic link to target.
func (GCSFs) Symlink(source, target string) error {
	return errors.New("403 symlinks are not supported")
}

// Chown changes the numeric uid and gid of the named file.
// Silently ignored.
func (GCSFs) Chown(name string, uid int, gid int) error {
	return nil
}

// Chmod changes the mode of the named file to mode.
// Silently ignored.
func (GCSFs) Chmod(name string, mode os.FileMode) error {
	return nil
}

// Chtimes changes the access and modification times of the named file.
// Silently ignored.
func (GCSFs) Chtimes(name string, atime, mtime time.Time) error {
	return errors.New("403 chtimes is not supported")
}

// ReadDir reads the directory named by dirname and returns
// a list of directory entries.
func (fs GCSFs) ReadDir(dirname string) ([]os.FileInfo, error) {
	var result []os.FileInfo
	// dirname must be already cleaned
	prefix := ""
	if len(dirname) > 0 && dirname != "." {
		prefix = strings.TrimPrefix(dirname, "/")
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
	}
	query := &storage.Query{Prefix: prefix, Delimiter: "/"}
	ctx, cancelFn := context.WithDeadline(context.Background(), time.Now().Add(fs.ctxTimeout))
	defer cancelFn()
	bkt := fs.svc.Bucket(fs.config.Bucket)
	it := bkt.Objects(ctx, query)
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			metrics.GCSListObjectsCompleted(err)
			return result, err
		}
		if len(attrs.Prefix) > 0 {
			name, _ := fs.resolve(attrs.Prefix, prefix)
			result = append(result, NewFileInfo(name, true, 0, time.Now()))
		} else {
			name, isDir := fs.resolve(attrs.Name, prefix)
			if len(name) == 0 {
				continue
			}
			if !attrs.Deleted.IsZero() {
				continue
			}
			fi := NewFileInfo(name, isDir, attrs.Size, attrs.Updated)
			if !isDir {
				fi.setContentType(attrs.ContentType)
			}
			result = append(result, fi)
		}
	}
	metrics.GCSListObjectsCompleted(nil)
	return result, nil
}

// IsUploadResumeSupported returns true if upload resume is supported.
// SFTP Resume is not supported on S3
func (GCSFs) IsUploadResumeSupported() bool {
	return false
}

// IsAtomicUploadSupported returns true if atomic upload is supported.
// S3 uploads are already atomic, we don't need to upload to a temporary
// file
func (GCSFs) IsAtomicUploadSupported() bool {
	return false
}

// IsNotExist returns a boolean indicating whether the error is known to
// report that a file or directory does not exist
func (GCSFs) IsNotExist(err error) bool {
	if err == nil {
		return false
	}
	if err == storage.ErrObjectNotExist || err == storage.ErrBucketNotExist {
		return true
	}
	if e, ok := err.(*googleapi.Error); ok {
		if e.Code == http.StatusNotFound {
			return true
		}
	}
	return strings.Contains(err.Error(), "404")
}

// IsPermission returns a boolean indicating whether the error is known to
// report that permission is denied.
func (GCSFs) IsPermission(err error) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*googleapi.Error); ok {
		if e.Code == http.StatusForbidden || e.Code == http.StatusUnauthorized {
			return true
		}
	}
	return strings.Contains(err.Error(), "403")
}

// CheckRootPath creates the specified local root directory if it does not exists
func (fs GCSFs) CheckRootPath(username string, uid int, gid int) bool {
	// we need a local directory for temporary files
	osFs := NewOsFs(fs.ConnectionID(), fs.localTempDir, nil)
	return osFs.CheckRootPath(username, uid, gid)
}

// ScanRootDirContents returns the number of files contained in the bucket,
// and their size
func (fs GCSFs) ScanRootDirContents() (int, int64, error) {
	numFiles := 0
	size := int64(0)
	query := &storage.Query{Prefix: fs.config.KeyPrefix}
	err := query.SetAttrSelection(gcsDefaultFieldsSelection)
	if err != nil {
		return numFiles, size, err
	}
	ctx, cancelFn := context.WithDeadline(context.Background(), time.Now().Add(fs.ctxLongTimeout))
	defer cancelFn()
	bkt := fs.svc.Bucket(fs.config.Bucket)
	it := bkt.Objects(ctx, query)
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			metrics.GCSListObjectsCompleted(err)
			return numFiles, size, err
		}
		if !attrs.Deleted.IsZero() {
			continue
		}
		numFiles++
		size += attrs.Size
	}
	metrics.GCSListObjectsCompleted(nil)
	return numFiles, size, err
}

// GetDirSize returns the number of files and the size for a folder
// including any subfolders
func (GCSFs) GetDirSize(dirname string) (int, int64, error) {
	return 0, 0, errUnsupported
}

// GetAtomicUploadPath returns the path to use for an atomic upload.
// S3 uploads are already atomic, we never call this method for S3
func (GCSFs) GetAtomicUploadPath(name string) string {
	return ""
}

// GetRelativePath returns the path for a file relative to the user's home dir.
// This is the path as seen by SFTP users
func (fs GCSFs) GetRelativePath(name string) string {
	rel := path.Clean(name)
	if rel == "." {
		rel = ""
	}
	if !path.IsAbs(rel) {
		rel = "/" + rel
	}
	if len(fs.config.KeyPrefix) > 0 {
		if !strings.HasPrefix(rel, "/"+fs.config.KeyPrefix) {
			rel = "/"
		}
		rel = path.Clean("/" + strings.TrimPrefix(rel, "/"+fs.config.KeyPrefix))
	}
	return rel
}

// Walk walks the file tree rooted at root, calling walkFn for each file or
// directory in the tree, including root
func (fs GCSFs) Walk(root string, walkFn filepath.WalkFunc) error {
	prefix := ""
	if len(root) > 0 && root != "." {
		prefix = strings.TrimPrefix(root, "/")
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
	}

	query := &storage.Query{Prefix: prefix}
	err := query.SetAttrSelection(gcsDefaultFieldsSelection)
	if err != nil {
		walkFn(root, nil, err) //nolint:errcheck
		return err
	}

	ctx, cancelFn := context.WithDeadline(context.Background(), time.Now().Add(fs.ctxTimeout))
	defer cancelFn()
	bkt := fs.svc.Bucket(fs.config.Bucket)
	it := bkt.Objects(ctx, query)
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			walkFn(root, nil, err) //nolint:errcheck
			metrics.GCSListObjectsCompleted(err)
			return err
		}
		if !attrs.Deleted.IsZero() {
			continue
		}
		isDir := strings.HasSuffix(attrs.Name, "/")
		name := path.Clean(attrs.Name)
		if len(name) == 0 {
			continue
		}
		err = walkFn(attrs.Name, NewFileInfo(name, isDir, attrs.Size, attrs.Updated), nil)
		if err != nil {
			break
		}
	}

	walkFn(root, NewFileInfo(root, true, 0, time.Now()), err) //nolint:errcheck
	metrics.GCSListObjectsCompleted(err)
	return err
}

// Join joins any number of path elements into a single path
func (GCSFs) Join(elem ...string) string {
	return strings.TrimPrefix(path.Join(elem...), "/")
}

// HasVirtualFolders returns true if folders are emulated
func (GCSFs) HasVirtualFolders() bool {
	return true
}

// ResolvePath returns the matching filesystem path for the specified sftp path
func (fs GCSFs) ResolvePath(sftpPath string) (string, error) {
	if !path.IsAbs(sftpPath) {
		sftpPath = path.Clean("/" + sftpPath)
	}
	return fs.Join(fs.config.KeyPrefix, strings.TrimPrefix(sftpPath, "/")), nil
}

func (fs *GCSFs) resolve(name string, prefix string) (string, bool) {
	result := strings.TrimPrefix(name, prefix)
	isDir := strings.HasSuffix(result, "/")
	if isDir {
		result = strings.TrimSuffix(result, "/")
	}
	return result, isDir
}

func (fs *GCSFs) isEqual(key string, sftpName string) bool {
	if key == sftpName {
		return true
	}
	if key == sftpName+"/" {
		return true
	}
	if key+"/" == sftpName {
		return true
	}
	return false
}

func (fs *GCSFs) checkIfBucketExists() error {
	ctx, cancelFn := context.WithDeadline(context.Background(), time.Now().Add(fs.ctxTimeout))
	defer cancelFn()
	bkt := fs.svc.Bucket(fs.config.Bucket)
	_, err := bkt.Attrs(ctx)
	metrics.GCSHeadBucketCompleted(err)
	return err
}

func (fs *GCSFs) getPrefixForStat(name string) string {
	prefix := path.Dir(name)
	if prefix == "/" || prefix == "." || len(prefix) == 0 {
		prefix = ""
	} else {
		prefix = strings.TrimPrefix(prefix, "/")
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
	}
	return prefix
}
