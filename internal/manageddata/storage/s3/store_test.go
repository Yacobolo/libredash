package s3_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Yacobolo/leapview/internal/manageddata/storage"
	manageds3 "github.com/Yacobolo/leapview/internal/manageddata/storage/s3"
	"github.com/Yacobolo/leapview/internal/manageddata/storage/storagetest"
	awsv4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestBlobStoreConformance(t *testing.T) {
	storagetest.BlobStoreConformance(t, func(t *testing.T) storage.BlobStore {
		client := newFakeClient()
		store, err := manageds3.New(client, &fakePresigner{}, manageds3.Config{Bucket: "private-data", Prefix: "managed"})
		if err != nil {
			t.Fatal(err)
		}
		return store
	})
}

func TestStoreUsesContentAddressedKeyAndStableURI(t *testing.T) {
	client := newFakeClient()
	store := newStore(t, client, &fakePresigner{})
	body := []byte("content addressed")
	blob := blobFor(body)
	stored, err := store.Put(t.Context(), blob, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	wantKey := "managed/blobs/sha256/" + blob.SHA256[:2] + "/" + blob.SHA256
	if client.lastPutKey != wantKey {
		t.Fatalf("PutObject key = %q, want %q", client.lastPutKey, wantKey)
	}
	if stored.URI != "s3://private-data/"+wantKey {
		t.Fatalf("URI = %q", stored.URI)
	}
	if client.lastPutIfNoneMatch != "*" {
		t.Fatalf("IfNoneMatch = %q", client.lastPutIfNoneMatch)
	}
}

func TestBlobInventoryPaginatesAndDeletesInOneBatch(t *testing.T) {
	client := newFakeClient()
	client.listPageSize = 1
	store := newStore(t, client, &fakePresigner{})
	first := blobFor([]byte("first"))
	second := blobFor([]byte("second"))
	for _, item := range []struct {
		blob storage.Blob
		body []byte
	}{{first, []byte("first")}, {second, []byte("second")}} {
		if _, err := store.Put(t.Context(), item.blob, bytes.NewReader(item.body)); err != nil {
			t.Fatal(err)
		}
	}
	var metadata []storage.BlobMetadata
	if err := store.WalkBlobs(t.Context(), func(item storage.BlobMetadata) error {
		metadata = append(metadata, item)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(metadata) != 2 || client.listCalls != 2 {
		t.Fatalf("WalkBlobs() = %#v, list calls = %d", metadata, client.listCalls)
	}
	for _, item := range metadata {
		if item.LastModified.IsZero() || item.Size <= 0 {
			t.Fatalf("invalid metadata = %#v", item)
		}
	}
	if err := store.DeleteBlobs(t.Context(), []string{first.SHA256, second.SHA256}); err != nil {
		t.Fatal(err)
	}
	if len(client.deleteBatches) != 1 || len(client.deleteBatches[0]) != 2 {
		t.Fatalf("delete batches = %#v", client.deleteBatches)
	}
	if err := store.DeleteBlobs(t.Context(), []string{first.SHA256, second.SHA256}); err != nil {
		t.Fatalf("idempotent DeleteBlobs() = %v", err)
	}
}

func TestBlobInventoryRejectsMalformedProviderKeys(t *testing.T) {
	client := newFakeClient()
	client.objects["managed/blobs/sha256/not-canonical"] = fakeObject{body: []byte("x"), modified: time.Now()}
	store := newStore(t, client, &fakePresigner{})
	if err := store.WalkBlobs(t.Context(), func(storage.BlobMetadata) error { return nil }); !errors.Is(err, storage.ErrIntegrity) {
		t.Fatalf("WalkBlobs() error = %v", err)
	}
}

func TestMultipartCreateSignCompleteAndAbort(t *testing.T) {
	client := newFakeClient()
	presigner := &fakePresigner{}
	store := newStore(t, client, presigner)
	body := []byte("multipart body")
	expected := blobFor(body)

	upload, err := store.CreateMultipart(t.Context(), expected)
	if err != nil {
		t.Fatal(err)
	}
	if upload.UploadID == "" || upload.Key == "" || upload.Existing {
		t.Fatalf("CreateMultipart() = %#v", upload)
	}
	partBody := body[:5]
	partDigest := blobFor(partBody).SHA256
	signed, err := store.SignPart(t.Context(), upload, storage.MultipartPartRequest{Number: 1, Size: int64(len(partBody)), SHA256: partDigest})
	if err != nil {
		t.Fatal(err)
	}
	if signed.URL != "https://uploads.example/part/1" || http.Header(signed.Headers).Get("X-Test") != "signed" {
		t.Fatalf("SignPart() = %#v", signed)
	}
	if presigner.lastChecksum != base64.StdEncoding.EncodeToString(mustDecodeHex(t, partDigest)) {
		t.Fatalf("signed checksum = %q", presigner.lastChecksum)
	}

	client.setMultipartBody(upload.UploadID, body)
	completed, err := store.CompleteMultipart(t.Context(), upload, []storage.CompletedMultipartPart{{Number: 1, ETag: "etag-1", SHA256: partDigest}})
	if err != nil {
		t.Fatal(err)
	}
	if completed.SHA256 != expected.SHA256 || completed.Size != expected.Size {
		t.Fatalf("CompleteMultipart() = %#v", completed)
	}
	completedAgain, err := store.CompleteMultipart(t.Context(), upload, []storage.CompletedMultipartPart{{Number: 1, ETag: "etag-1", SHA256: partDigest}})
	if err != nil || completedAgain != completed {
		t.Fatalf("idempotent CompleteMultipart() = %#v, %v", completedAgain, err)
	}
	if err := store.AbortMultipart(t.Context(), upload); err != nil {
		t.Fatal(err)
	}
	if err := store.AbortMultipart(t.Context(), upload); err != nil {
		t.Fatalf("idempotent AbortMultipart() = %v", err)
	}
}

func TestMultipartCompletionDeletesContentThatFailsStreamVerification(t *testing.T) {
	client := newFakeClient()
	store := newStore(t, client, &fakePresigner{})
	expected := blobFor([]byte("expected"))
	upload, err := store.CreateMultipart(t.Context(), expected)
	if err != nil {
		t.Fatal(err)
	}
	client.setMultipartBody(upload.UploadID, []byte("tampered"))
	_, err = store.CompleteMultipart(t.Context(), upload, []storage.CompletedMultipartPart{{Number: 1, ETag: "etag"}})
	if !errors.Is(err, storage.ErrIntegrity) {
		t.Fatalf("CompleteMultipart() error = %v", err)
	}
	if _, exists := client.object(upload.Key); exists {
		t.Fatal("failed multipart object was not deleted")
	}
}

func TestS3ErrorsDoNotExposeCredentials(t *testing.T) {
	client := newFakeClient()
	client.failure = errors.New("request failed: X-Amz-Credential=AKIA_SECRET&X-Amz-Signature=SECRET")
	store := newStore(t, client, &fakePresigner{})
	_, err := store.Stat(t.Context(), blobFor([]byte("missing")).SHA256)
	if err == nil || strings.Contains(err.Error(), "SECRET") || !errors.Is(err, storage.ErrBackend) {
		t.Fatalf("sanitized error = %v", err)
	}
}

func newStore(t *testing.T, client *fakeClient, presigner *fakePresigner) *manageds3.Store {
	t.Helper()
	store, err := manageds3.New(client, presigner, manageds3.Config{Bucket: "private-data", Prefix: "/managed/", SignExpiry: 10 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

type fakeClient struct {
	mu                 sync.Mutex
	objects            map[string]fakeObject
	multipart          map[string]fakeMultipart
	nextUpload         int
	failure            error
	lastPutKey         string
	lastPutIfNoneMatch string
	listPageSize       int
	listCalls          int
	deleteBatches      [][]string
}

type fakeObject struct {
	body     []byte
	metadata map[string]string
	modified time.Time
}

type fakeMultipart struct {
	key      string
	metadata map[string]string
	body     []byte
}

func newFakeClient() *fakeClient {
	return &fakeClient{objects: map[string]fakeObject{}, multipart: map[string]fakeMultipart{}}
}

func (c *fakeClient) PutObject(_ context.Context, input *awss3.PutObjectInput, _ ...func(*awss3.Options)) (*awss3.PutObjectOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failure != nil {
		return nil, c.failure
	}
	key := dereference(input.Key)
	c.lastPutKey = key
	c.lastPutIfNoneMatch = dereference(input.IfNoneMatch)
	if _, exists := c.objects[key]; exists && dereference(input.IfNoneMatch) == "*" {
		return nil, fakeAPIError{code: "PreconditionFailed"}
	}
	body, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	if input.ContentLength != nil && *input.ContentLength != int64(len(body)) {
		return nil, fakeAPIError{code: "IncompleteBody"}
	}
	sum := sha256.Sum256(body)
	if input.ChecksumSHA256 != nil && *input.ChecksumSHA256 != base64.StdEncoding.EncodeToString(sum[:]) {
		return nil, fakeAPIError{code: "BadDigest"}
	}
	c.objects[key] = fakeObject{body: append([]byte(nil), body...), metadata: clone(input.Metadata), modified: time.Now().UTC()}
	return &awss3.PutObjectOutput{}, nil
}

func (c *fakeClient) HeadObject(_ context.Context, input *awss3.HeadObjectInput, _ ...func(*awss3.Options)) (*awss3.HeadObjectOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failure != nil {
		return nil, c.failure
	}
	object, exists := c.objects[dereference(input.Key)]
	if !exists {
		return nil, fakeAPIError{code: "NotFound"}
	}
	length := int64(len(object.body))
	return &awss3.HeadObjectOutput{ContentLength: &length, Metadata: clone(object.metadata)}, nil
}

func (c *fakeClient) GetObject(_ context.Context, input *awss3.GetObjectInput, _ ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failure != nil {
		return nil, c.failure
	}
	object, exists := c.objects[dereference(input.Key)]
	if !exists {
		return nil, fakeAPIError{code: "NoSuchKey"}
	}
	return &awss3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(object.body))}, nil
}

func (c *fakeClient) ListObjectsV2(_ context.Context, input *awss3.ListObjectsV2Input, _ ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failure != nil {
		return nil, c.failure
	}
	c.listCalls++
	var keys []string
	for key := range c.objects {
		if strings.HasPrefix(key, dereference(input.Prefix)) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	start := 0
	if token := dereference(input.ContinuationToken); token != "" {
		parsed, err := strconv.Atoi(token)
		if err != nil {
			return nil, err
		}
		start = parsed
	}
	pageSize := c.listPageSize
	if pageSize <= 0 {
		pageSize = len(keys)
	}
	end := min(start+pageSize, len(keys))
	result := &awss3.ListObjectsV2Output{}
	for _, key := range keys[start:end] {
		object := c.objects[key]
		size := int64(len(object.body))
		modified := object.modified
		result.Contents = append(result.Contents, types.Object{Key: testPointer(key), Size: &size, LastModified: &modified})
	}
	if end < len(keys) {
		result.IsTruncated = testPointer(true)
		result.NextContinuationToken = testPointer(strconv.Itoa(end))
	}
	return result, nil
}

func (c *fakeClient) DeleteObjects(_ context.Context, input *awss3.DeleteObjectsInput, _ ...func(*awss3.Options)) (*awss3.DeleteObjectsOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failure != nil {
		return nil, c.failure
	}
	batch := make([]string, 0, len(input.Delete.Objects))
	for _, object := range input.Delete.Objects {
		key := dereference(object.Key)
		batch = append(batch, key)
		delete(c.objects, key)
	}
	c.deleteBatches = append(c.deleteBatches, batch)
	return &awss3.DeleteObjectsOutput{}, nil
}

func (c *fakeClient) CreateMultipartUpload(_ context.Context, input *awss3.CreateMultipartUploadInput, _ ...func(*awss3.Options)) (*awss3.CreateMultipartUploadOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failure != nil {
		return nil, c.failure
	}
	c.nextUpload++
	id := fmt.Sprintf("upload-%d", c.nextUpload)
	c.multipart[id] = fakeMultipart{key: dereference(input.Key), metadata: clone(input.Metadata)}
	return &awss3.CreateMultipartUploadOutput{UploadId: &id}, nil
}

func (c *fakeClient) CompleteMultipartUpload(_ context.Context, input *awss3.CompleteMultipartUploadInput, _ ...func(*awss3.Options)) (*awss3.CompleteMultipartUploadOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failure != nil {
		return nil, c.failure
	}
	id := dereference(input.UploadId)
	upload, exists := c.multipart[id]
	if !exists {
		return nil, fakeAPIError{code: "NoSuchUpload"}
	}
	if _, exists := c.objects[upload.key]; exists && dereference(input.IfNoneMatch) == "*" {
		return nil, fakeAPIError{code: "PreconditionFailed"}
	}
	c.objects[upload.key] = fakeObject{body: append([]byte(nil), upload.body...), metadata: clone(upload.metadata), modified: time.Now().UTC()}
	delete(c.multipart, id)
	return &awss3.CompleteMultipartUploadOutput{}, nil
}

func (c *fakeClient) AbortMultipartUpload(_ context.Context, input *awss3.AbortMultipartUploadInput, _ ...func(*awss3.Options)) (*awss3.AbortMultipartUploadOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failure != nil {
		return nil, c.failure
	}
	id := dereference(input.UploadId)
	if _, exists := c.multipart[id]; !exists {
		return nil, fakeAPIError{code: "NoSuchUpload"}
	}
	delete(c.multipart, id)
	return &awss3.AbortMultipartUploadOutput{}, nil
}

func (c *fakeClient) setMultipartBody(uploadID string, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	upload := c.multipart[uploadID]
	upload.body = append([]byte(nil), body...)
	c.multipart[uploadID] = upload
}

func (c *fakeClient) object(key string) (fakeObject, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	object, exists := c.objects[key]
	return object, exists
}

type fakePresigner struct {
	lastChecksum string
}

func (p *fakePresigner) PresignUploadPart(_ context.Context, input *awss3.UploadPartInput, _ ...func(*awss3.PresignOptions)) (*awsv4.PresignedHTTPRequest, error) {
	p.lastChecksum = dereference(input.ChecksumSHA256)
	headers := http.Header{}
	headers.Set("X-Test", "signed")
	return &awsv4.PresignedHTTPRequest{URL: fmt.Sprintf("https://uploads.example/part/%d", dereference(input.PartNumber)), SignedHeader: headers}, nil
}

type fakeAPIError struct{ code string }

func (e fakeAPIError) Error() string     { return e.code }
func (e fakeAPIError) ErrorCode() string { return e.code }

func blobFor(body []byte) storage.Blob {
	sum := sha256.Sum256(body)
	return storage.Blob{SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body))}
}

func mustDecodeHex(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func dereference[T any](value *T) T {
	if value == nil {
		var zero T
		return zero
	}
	return *value
}

func clone(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func testPointer[T any](value T) *T { return &value }

var _ manageds3.Client = (*fakeClient)(nil)
var _ manageds3.PartPresigner = (*fakePresigner)(nil)
var _ manageds3.Client = (*awss3.Client)(nil)
var _ manageds3.PartPresigner = (*awss3.PresignClient)(nil)
