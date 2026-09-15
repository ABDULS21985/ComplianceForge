package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/complianceforge/platform/internal/config"
)

func TestNewConfiguredServiceBuildsLocalStore(t *testing.T) {
	root := filepath.Join(t.TempDir(), "objects")
	service, err := NewConfiguredService(context.Background(), config.StorageConfig{Type: "local", Path: root})
	if err != nil {
		t.Fatalf("NewConfiguredService() error = %v", err)
	}
	if _, ok := service.(*LocalStorageService); !ok {
		t.Fatalf("service type = %T", service)
	}
}

func TestNewConfiguredServiceBuildsS3WithoutStaticApplicationCredentials(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	service, err := NewConfiguredService(context.Background(), config.StorageConfig{
		Type:                  "s3",
		S3Bucket:              "evidence",
		S3Region:              "eu-west-2",
		S3Endpoint:            "http://127.0.0.1:9000",
		S3ForcePathStyle:      true,
		S3KMSKeyID:            "test-kms-key",
		S3ExpectedBucketOwner: "123456789012",
	})
	if err != nil {
		t.Fatalf("NewConfiguredService() error = %v", err)
	}
	s3Service, ok := service.(*S3StorageService)
	if !ok || s3Service.bucket != "evidence" || s3Service.kmsKeyID == nil || s3Service.expectedBucketOwner == nil {
		t.Fatalf("service = %#v", service)
	}
}

func TestNewConfiguredServiceRejectsUnknownType(t *testing.T) {
	if _, err := NewConfiguredService(context.Background(), config.StorageConfig{Type: "ftp"}); err == nil {
		t.Fatal("NewConfiguredService() error = nil")
	}
}
