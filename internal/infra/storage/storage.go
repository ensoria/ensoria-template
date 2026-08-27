// Package storage wires the file storage backends into the DI container.
//
// It composes two file.FileSystem backends — a local disk (filelocal) and an
// S3 disk (files3, pointed at MinIO for local development) — into a single
// file.Storage registry. Switching between backends is done through Storage;
// the disk named by DefaultDisk is also exposed directly as file.FileSystem so
// controllers can inject one FileSystem without knowing about disks.
package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/file/pkg/file"
	"github.com/ensoria/file/pkg/filelocal"
	"github.com/ensoria/file/pkg/files3"
	"github.com/ensoria/loggear/pkg/loggear"
)

// Storage configuration. Values are hardcoded for now; they will be derived
// from registry.ModuleParams() once the config package supports storage.
const (
	// Disk names registered in the Storage registry. They are exported because
	// they are what a caller passes to Storage.Disk, and because describe's stub
	// registry mirrors them: the names have to be declared once, or the two
	// registries drift apart without anything reporting it.
	DiskLocal = "local"
	DiskS3    = "s3"

	// DefaultDisk selects the backend exposed by Storage.Default() and injected
	// as file.FileSystem. Switch the default backend here.
	// TODO: configパッケージを使って設定を取得するようにする
	// そのときは describe のスタブ registry (bootstrap/describe/stubs.go の
	// stubStorage) の前提も見直すこと。ディスク名が定数でなくなると、
	// 名前を写して揃えるという今の作りが成り立たなくなる。
	DefaultDisk = DiskS3

	// Local disk: root directory (created if missing by filelocal.New).
	localRoot = "./.storage/local"

	// S3 disk (MinIO for local dev). Endpoint / path-style / checksum settings
	// match compose.yaml's minio service.
	s3Endpoint  = "http://localhost:9000"
	s3Region    = "us-east-1"
	s3Bucket    = "ensoria"
	s3KeyPrefix = "uploads"
	s3AccessKey = "minioadmin"
	s3SecretKey = "minioadmin"
)

// NewDefaultStorage builds the file.Storage registry with a local disk and an
// S3 disk, exposing DefaultDisk as the default. The local disk owns a directory
// fd (filelocal implements io.Closer) and is closed on shutdown; the S3 disk
// borrows the injected client and needs no cleanup.
func NewDefaultStorage(envVal *string) func(lc dikit.LC) (file.Storage, error) {
	return func(lc dikit.LC) (file.Storage, error) {
		// TODO: configパッケージを使って設定を取得するようにする
		local, err := filelocal.New(localRoot)
		if err != nil {
			return nil, fmt.Errorf("local disk init failed: %w", err)
		}

		// Retain the client so OnStart can verify connectivity via HeadBucket.
		s3Client := newS3Client()
		s3fs := files3.New(s3Client, s3Bucket, s3KeyPrefix)

		storage, err := file.NewStorage(
			file.WithDisk(DiskLocal, local),
			file.WithDisk(DiskS3, s3fs),
			file.WithDefault(DefaultDisk),
		)
		if err != nil {
			return nil, fmt.Errorf("storage init failed: %w", err)
		}

		lc.Append(dikit.Hook{
			OnStart: func(ctx context.Context) error {
				// Verify S3 connectivity and that the target bucket exists and is
				// reachable (HeadBucket). Like the other infra connections, this
				// fails startup when the backend is unavailable. Because DefaultDisk
				// is S3, MinIO and the bucket must be up before the app starts.
				if _, err := s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
					Bucket: aws.String(s3Bucket),
				}); err != nil {
					return fmt.Errorf("S3 bucket check failed (bucket=%q): %w", s3Bucket, err)
				}
				loggear.Info("Storage connection verified", "default", DefaultDisk, "disks", storage.Names())
				return nil
			},
			OnStop: func(ctx context.Context) error {
				loggear.Info("Shutting down storage")
				// Only filelocal owns a closable resource (its directory fd).
				if closer, ok := local.(io.Closer); ok {
					return closer.Close()
				}
				return nil
			},
		})

		return storage, nil
	}
}

// NewDefaultFileSystem exposes Storage's default disk as file.FileSystem so
// callers can inject a single FileSystem. It depends on the file.Storage
// provided by NewDefaultStorage.
func NewDefaultFileSystem(storage file.Storage) (file.FileSystem, error) {
	fsys := storage.Default()
	if fsys == nil {
		return nil, fmt.Errorf("storage has no default disk")
	}
	return fsys, nil
}

// newS3Client builds an S3 client for an S3-compatible store (MinIO): static
// credentials, a custom BaseEndpoint, path-style addressing, and checksum
// settings tuned for MinIO compatibility.
func newS3Client() *s3.Client {
	return s3.New(s3.Options{
		Region:                     s3Region,
		BaseEndpoint:               aws.String(s3Endpoint),
		Credentials:                credentials.NewStaticCredentialsProvider(s3AccessKey, s3SecretKey, ""),
		UsePathStyle:               true,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
}
