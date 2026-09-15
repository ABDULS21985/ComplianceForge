package storage

import (
	"context"
	"fmt"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	appconfig "github.com/complianceforge/platform/internal/config"
)

// NewConfiguredService constructs the configured private object store. AWS
// credentials come from the SDK's standard workload-identity/default chain;
// static credentials are intentionally not part of application configuration.
func NewConfiguredService(ctx context.Context, configured appconfig.StorageConfig) (StorageService, error) {
	switch strings.ToLower(strings.TrimSpace(configured.Type)) {
	case "local":
		return NewLocalStorageService(configured.Path)
	case "s3":
		awsConfiguration, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(configured.S3Region))
		if err != nil {
			return nil, fmt.Errorf("load AWS object-storage configuration: %w", err)
		}
		client := s3.NewFromConfig(awsConfiguration, func(options *s3.Options) {
			options.UsePathStyle = configured.S3ForcePathStyle
			if endpoint := strings.TrimSpace(configured.S3Endpoint); endpoint != "" {
				options.BaseEndpoint = &endpoint
			}
		})
		return NewS3StorageService(client, configured.S3Bucket,
			WithKMSKey(configured.S3KMSKeyID),
			WithExpectedBucketOwner(configured.S3ExpectedBucketOwner),
		)
	default:
		return nil, fmt.Errorf("unsupported storage type %q", configured.Type)
	}
}
