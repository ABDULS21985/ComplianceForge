package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"path"
	"strings"
	"time"
	"unicode"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const maximumSignedDownloadLifetime = 15 * time.Minute

var ErrUnsignedObject = errors.New("object is not eligible for a signed download")

type s3ObjectAPI interface {
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

type s3PresignAPI interface {
	PresignGetObject(context.Context, *s3.GetObjectInput, ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
}

// S3StorageService stores only private objects. It enables transport checksums,
// server-side encryption, and short-lived download URLs; callers retain the
// object key rather than a long-lived or public URL.
type S3StorageService struct {
	client              s3ObjectAPI
	presigner           s3PresignAPI
	bucket              string
	expectedBucketOwner *string
	kmsKeyID            *string
}

type S3Option func(*S3StorageService)

func WithExpectedBucketOwner(accountID string) S3Option {
	return func(service *S3StorageService) {
		if trimmed := strings.TrimSpace(accountID); trimmed != "" {
			service.expectedBucketOwner = aws.String(trimmed)
		}
	}
}

func WithKMSKey(keyID string) S3Option {
	return func(service *S3StorageService) {
		if trimmed := strings.TrimSpace(keyID); trimmed != "" {
			service.kmsKeyID = aws.String(trimmed)
		}
	}
}

func NewS3StorageService(client *s3.Client, bucket string, options ...S3Option) (*S3StorageService, error) {
	if client == nil {
		return nil, errors.New("s3 client is required")
	}
	return newS3StorageService(client, s3.NewPresignClient(client), bucket, options...)
}

func newS3StorageService(client s3ObjectAPI, presigner s3PresignAPI, bucket string, options ...S3Option) (*S3StorageService, error) {
	bucket = strings.TrimSpace(bucket)
	if client == nil || presigner == nil || bucket == "" {
		return nil, errors.New("s3 client, presigner, and bucket are required")
	}
	service := &S3StorageService{client: client, presigner: presigner, bucket: bucket}
	for _, option := range options {
		option(service)
	}
	return service, nil
}

func (s *S3StorageService) Upload(ctx context.Context, key string, data io.Reader) (string, error) {
	key, err := normalizeObjectKey(key)
	if err != nil {
		return "", err
	}
	if data == nil {
		return "", errors.New("upload data is required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	input := &s3.PutObjectInput{
		Bucket:              aws.String(s.bucket),
		Key:                 aws.String(key),
		Body:                data,
		ChecksumAlgorithm:   types.ChecksumAlgorithmSha256,
		ContentType:         aws.String("application/octet-stream"),
		CacheControl:        aws.String("private, no-store"),
		ExpectedBucketOwner: s.expectedBucketOwner,
		Metadata: map[string]string{
			"complianceforge-state": objectState(key),
		},
		IfNoneMatch: aws.String("*"),
	}
	if seeker, ok := data.(io.Seeker); ok {
		current, seekErr := seeker.Seek(0, io.SeekCurrent)
		if seekErr == nil {
			end, endErr := seeker.Seek(0, io.SeekEnd)
			if endErr == nil && end >= current {
				input.ContentLength = aws.Int64(end - current)
			}
			_, _ = seeker.Seek(current, io.SeekStart)
		}
	}
	if s.kmsKeyID != nil {
		input.ServerSideEncryption = types.ServerSideEncryptionAwsKms
		input.SSEKMSKeyId = s.kmsKeyID
		input.BucketKeyEnabled = aws.Bool(true)
	} else {
		input.ServerSideEncryption = types.ServerSideEncryptionAes256
	}
	if _, err := s.client.PutObject(ctx, input); err != nil {
		return "", fmt.Errorf("upload s3 object: %w", err)
	}
	return key, nil
}

func (s *S3StorageService) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	key, err := normalizeObjectKey(key)
	if err != nil {
		return nil, err
	}
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket:              aws.String(s.bucket),
		Key:                 aws.String(key),
		ChecksumMode:        types.ChecksumModeEnabled,
		ExpectedBucketOwner: s.expectedBucketOwner,
	})
	if err != nil {
		return nil, fmt.Errorf("download s3 object: %w", err)
	}
	if output == nil || output.Body == nil {
		return nil, errors.New("s3 returned an empty object response")
	}
	return output.Body, nil
}

func (s *S3StorageService) Verify(ctx context.Context, key, expectedSHA256 string, expectedSize int64) error {
	reader, err := s.Download(ctx, key)
	if err != nil {
		return err
	}
	return verifyObject(ctx, reader, expectedSHA256, expectedSize)
}

func (s *S3StorageService) Delete(ctx context.Context, key string) error {
	key, err := normalizeObjectKey(key)
	if err != nil {
		return err
	}
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket:              aws.String(s.bucket),
		Key:                 aws.String(key),
		ExpectedBucketOwner: s.expectedBucketOwner,
	}); err != nil {
		return fmt.Errorf("delete s3 object: %w", err)
	}
	return nil
}

// PresignDownload creates a narrowly scoped URL only for promoted evidence
// objects. Quarantine keys can never be exposed through this method.
func (s *S3StorageService) PresignDownload(ctx context.Context, key, filename string, lifetime time.Duration) (string, error) {
	key, err := normalizeObjectKey(key)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(key, "evidence/") {
		return "", ErrUnsignedObject
	}
	filename, err = safeDownloadFilename(filename)
	if err != nil {
		return "", err
	}
	if lifetime <= 0 || lifetime > maximumSignedDownloadLifetime {
		lifetime = maximumSignedDownloadLifetime
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
	request, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket:                     aws.String(s.bucket),
		Key:                        aws.String(key),
		ExpectedBucketOwner:        s.expectedBucketOwner,
		ResponseCacheControl:       aws.String("private, no-store"),
		ResponseContentDisposition: aws.String(disposition),
		ResponseContentType:        aws.String("application/octet-stream"),
	}, func(options *s3.PresignOptions) {
		options.Expires = lifetime
	})
	if err != nil {
		return "", fmt.Errorf("sign s3 download: %w", err)
	}
	if request == nil || strings.TrimSpace(request.URL) == "" {
		return "", errors.New("s3 returned an empty signed download")
	}
	return request.URL, nil
}

func normalizeObjectKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || strings.ContainsRune(value, '\x00') {
		return "", ErrInvalidPath
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != value {
		return "", ErrInvalidPath
	}
	for _, character := range clean {
		if unicode.IsControl(character) {
			return "", ErrInvalidPath
		}
	}
	return clean, nil
}

func safeDownloadFilename(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 255 || value != path.Base(strings.ReplaceAll(value, "\\", "/")) {
		return "", ErrInvalidPath
	}
	for _, character := range value {
		if unicode.IsControl(character) || character == '/' || character == '\\' {
			return "", ErrInvalidPath
		}
	}
	return value, nil
}

func objectState(key string) string {
	if strings.HasPrefix(key, "quarantine/") {
		return "quarantine"
	}
	return "clean"
}

var _ StorageService = (*S3StorageService)(nil)
