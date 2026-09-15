package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type fakeS3ObjectAPI struct {
	putInput    *s3.PutObjectInput
	getInput    *s3.GetObjectInput
	deleteInput *s3.DeleteObjectInput
	putErr      error
	getErr      error
	deleteErr   error
	body        io.ReadCloser
}

func (f *fakeS3ObjectAPI) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.putInput = input
	return &s3.PutObjectOutput{}, f.putErr
}

func (f *fakeS3ObjectAPI) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	f.getInput = input
	if f.getErr != nil {
		return nil, f.getErr
	}
	return &s3.GetObjectOutput{Body: f.body}, nil
}

func (f *fakeS3ObjectAPI) DeleteObject(_ context.Context, input *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	f.deleteInput = input
	return &s3.DeleteObjectOutput{}, f.deleteErr
}

type fakeS3Presigner struct {
	input   *s3.GetObjectInput
	expires time.Duration
	result  *v4.PresignedHTTPRequest
	err     error
}

func (f *fakeS3Presigner) PresignGetObject(_ context.Context, input *s3.GetObjectInput, options ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error) {
	f.input = input
	configuration := &s3.PresignOptions{}
	for _, option := range options {
		option(configuration)
	}
	f.expires = configuration.Expires
	return f.result, f.err
}

func newTestS3Storage(t *testing.T, client *fakeS3ObjectAPI, presigner *fakeS3Presigner, options ...S3Option) *S3StorageService {
	t.Helper()
	service, err := newS3StorageService(client, presigner, "evidence-bucket", options...)
	if err != nil {
		t.Fatalf("newS3StorageService() error = %v", err)
	}
	return service
}

func TestS3StorageUploadUsesPrivateEncryptedChecksummedObject(t *testing.T) {
	client := &fakeS3ObjectAPI{}
	service := newTestS3Storage(t, client, &fakeS3Presigner{}, WithExpectedBucketOwner("123456789012"))
	payload := bytes.NewReader([]byte("evidence"))
	key, err := service.Upload(context.Background(), "quarantine/tenant/object", payload)
	if err != nil || key != "quarantine/tenant/object" {
		t.Fatalf("Upload() key=%q error=%v", key, err)
	}
	input := client.putInput
	if input == nil || input.Bucket == nil || *input.Bucket != "evidence-bucket" || input.Key == nil || *input.Key != key {
		t.Fatalf("PutObject input = %#v", input)
	}
	if input.ChecksumAlgorithm != types.ChecksumAlgorithmSha256 || input.ServerSideEncryption != types.ServerSideEncryptionAes256 {
		t.Fatalf("checksum=%q encryption=%q", input.ChecksumAlgorithm, input.ServerSideEncryption)
	}
	if input.ContentLength == nil || *input.ContentLength != 8 || input.CacheControl == nil || *input.CacheControl != "private, no-store" {
		t.Fatalf("content length/cache = %#v/%#v", input.ContentLength, input.CacheControl)
	}
	if input.Metadata["complianceforge-state"] != "quarantine" || input.ExpectedBucketOwner == nil || *input.ExpectedBucketOwner != "123456789012" {
		t.Fatalf("metadata/owner = %#v/%#v", input.Metadata, input.ExpectedBucketOwner)
	}
	if input.IfNoneMatch == nil || *input.IfNoneMatch != "*" {
		t.Fatalf("IfNoneMatch = %#v", input.IfNoneMatch)
	}
}

func TestS3StorageUploadSupportsKMS(t *testing.T) {
	client := &fakeS3ObjectAPI{}
	service := newTestS3Storage(t, client, &fakeS3Presigner{}, WithKMSKey("arn:aws:kms:region:account:key/id"))
	if _, err := service.Upload(context.Background(), "evidence/tenant/object", strings.NewReader("safe")); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	input := client.putInput
	if input.ServerSideEncryption != types.ServerSideEncryptionAwsKms || input.SSEKMSKeyId == nil || input.BucketKeyEnabled == nil || !*input.BucketKeyEnabled {
		t.Fatalf("KMS PutObject input = %#v", input)
	}
	if input.Metadata["complianceforge-state"] != "clean" {
		t.Fatalf("state metadata = %q", input.Metadata["complianceforge-state"])
	}
}

func TestS3StorageDownloadAndDelete(t *testing.T) {
	client := &fakeS3ObjectAPI{body: io.NopCloser(strings.NewReader("evidence"))}
	service := newTestS3Storage(t, client, &fakeS3Presigner{}, WithExpectedBucketOwner("123"))
	reader, err := service.Download(context.Background(), "evidence/tenant/object")
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	contents, _ := io.ReadAll(reader)
	_ = reader.Close()
	if string(contents) != "evidence" || client.getInput.ChecksumMode != types.ChecksumModeEnabled {
		t.Fatalf("Download() contents=%q input=%#v", contents, client.getInput)
	}
	if err := service.Delete(context.Background(), "evidence/tenant/object"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if client.deleteInput == nil || *client.deleteInput.Key != "evidence/tenant/object" {
		t.Fatalf("DeleteObject input = %#v", client.deleteInput)
	}
}

func TestS3StorageIntegrityVerificationUsesChecksummedPrivateRead(t *testing.T) {
	payload := []byte("evidence")
	digest := sha256.Sum256(payload)
	client := &fakeS3ObjectAPI{body: io.NopCloser(bytes.NewReader(payload))}
	service := newTestS3Storage(t, client, &fakeS3Presigner{}, WithExpectedBucketOwner("123456789012"))
	if err := service.Verify(context.Background(), "evidence/tenant/object", hex.EncodeToString(digest[:]), int64(len(payload))); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if client.getInput == nil || client.getInput.ChecksumMode != types.ChecksumModeEnabled || client.getInput.ExpectedBucketOwner == nil {
		t.Fatalf("verification GetObject input = %#v", client.getInput)
	}
}

func TestS3StoragePresignsOnlyCleanObjectsWithSafeHeaders(t *testing.T) {
	presigner := &fakeS3Presigner{result: &v4.PresignedHTTPRequest{URL: "https://example.test/download", Method: http.MethodGet}}
	service := newTestS3Storage(t, &fakeS3ObjectAPI{}, presigner)
	url, err := service.PresignDownload(context.Background(), "evidence/tenant/object", "control evidence.pdf", time.Hour)
	if err != nil || url != "https://example.test/download" {
		t.Fatalf("PresignDownload() url=%q error=%v", url, err)
	}
	if presigner.expires != maximumSignedDownloadLifetime {
		t.Fatalf("expiry=%s, want %s", presigner.expires, maximumSignedDownloadLifetime)
	}
	if presigner.input.ResponseContentType == nil || *presigner.input.ResponseContentType != "application/octet-stream" ||
		presigner.input.ResponseCacheControl == nil || *presigner.input.ResponseCacheControl != "private, no-store" ||
		presigner.input.ResponseContentDisposition == nil || !strings.HasPrefix(*presigner.input.ResponseContentDisposition, "attachment;") || strings.ContainsAny(*presigner.input.ResponseContentDisposition, "\r\n") {
		t.Fatalf("unsafe signed response headers: %#v", presigner.input)
	}

	for _, test := range []struct{ key, filename string }{
		{"quarantine/tenant/object", "evidence.pdf"},
		{"evidence/tenant/object", "../secret.pdf"},
		{"evidence/tenant/object", "bad\r\nname.pdf"},
	} {
		if _, err := service.PresignDownload(context.Background(), test.key, test.filename, time.Minute); err == nil {
			t.Fatalf("PresignDownload(%q, %q) error = nil", test.key, test.filename)
		}
	}
}

func TestS3StorageRejectsInvalidKeysBeforeCallingS3(t *testing.T) {
	client := &fakeS3ObjectAPI{}
	service := newTestS3Storage(t, client, &fakeS3Presigner{})
	for _, key := range []string{"", "/absolute", "../outside", "a/../outside", "a//b", `a\b`, "bad\x00key", "bad\rkey"} {
		if _, err := service.Upload(context.Background(), key, strings.NewReader("x")); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("Upload(%q) error = %v", key, err)
		}
	}
	if client.putInput != nil {
		t.Fatal("invalid key reached S3 client")
	}
}

func TestNewS3StorageRequiresDependencies(t *testing.T) {
	client := &fakeS3ObjectAPI{}
	presigner := &fakeS3Presigner{}
	for _, test := range []struct {
		client    s3ObjectAPI
		presigner s3PresignAPI
		bucket    string
	}{{nil, presigner, "bucket"}, {client, nil, "bucket"}, {client, presigner, " "}} {
		if _, err := newS3StorageService(test.client, test.presigner, test.bucket); err == nil {
			t.Fatal("newS3StorageService() error = nil")
		}
	}
}
