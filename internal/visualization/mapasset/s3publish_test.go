package mapasset

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

func TestS3PublicationStoreUsesImmutableConditionalObjects(t *testing.T) {
	client := &fakeS3PublicationClient{}
	store, err := NewS3PublicationStore(client, S3PublicationConfig{Bucket: "map-assets", Prefix: "/production/maps/"})
	if err != nil {
		t.Fatal(err)
	}
	object := PublicationObject{Key: "leapview/archive.pmtiles", Digest: "2d97ee8907670936ab722da7ca06eafec0734392f73fa1cd337d4debd85d676f", Size: 4, ContentType: "application/vnd.pmtiles", CacheControl: ImmutableCacheControl}
	if err := store.Put(context.Background(), object, bytes.NewReader([]byte("data"))); err != nil {
		t.Fatal(err)
	}
	input := client.put
	if aws.ToString(input.Bucket) != "map-assets" || aws.ToString(input.Key) != "production/maps/leapview/archive.pmtiles" || aws.ToString(input.IfNoneMatch) != "*" {
		t.Fatalf("put destination = %#v", input)
	}
	if aws.ToString(input.CacheControl) != ImmutableCacheControl || aws.ToString(input.ContentType) != object.ContentType || input.Metadata["sha256"] != object.Digest || input.ServerSideEncryption != types.ServerSideEncryptionAes256 {
		t.Fatalf("put metadata = %#v", input)
	}
	if aws.ToString(input.ChecksumSHA256) == "" || aws.ToInt64(input.ContentLength) != 4 {
		t.Fatalf("put integrity fields = %#v", input)
	}
	body, err := io.ReadAll(input.Body)
	if err != nil || string(body) != "data" {
		t.Fatalf("put body = %q, error = %v", body, err)
	}
}

func TestS3PublicationStoreStatsDigestAndMapsNotFound(t *testing.T) {
	client := &fakeS3PublicationClient{head: &awss3.HeadObjectOutput{ContentLength: aws.Int64(9), Metadata: map[string]string{"sha256": "abc"}}}
	store, err := NewS3PublicationStore(client, S3PublicationConfig{Bucket: "map-assets", Prefix: "maps"})
	if err != nil {
		t.Fatal(err)
	}
	object, err := store.Stat(context.Background(), "asset.json")
	if err != nil || object.Digest != "abc" || object.Size != 9 || object.Key != "asset.json" {
		t.Fatalf("Stat() = %#v, %v", object, err)
	}
	client.headErr = &smithy.GenericAPIError{Code: "NotFound", Message: "missing"}
	if _, err := store.Stat(context.Background(), "missing.json"); !errors.Is(err, ErrPublicationObjectNotFound) {
		t.Fatalf("Stat() error = %v, want not found", err)
	}
}

func TestS3PublicationStoreRejectsUnsafeConfigurationAndKeys(t *testing.T) {
	client := &fakeS3PublicationClient{}
	for _, config := range []S3PublicationConfig{{}, {Bucket: "bad/name"}, {Bucket: "ok", Prefix: "../escape"}} {
		if _, err := NewS3PublicationStore(client, config); err == nil {
			t.Fatalf("NewS3PublicationStore(%#v) accepted", config)
		}
	}
	store, err := NewS3PublicationStore(client, S3PublicationConfig{Bucket: "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Stat(context.Background(), "../secret"); err == nil {
		t.Fatal("unsafe publication key accepted")
	}
}

type fakeS3PublicationClient struct {
	put     *awss3.PutObjectInput
	head    *awss3.HeadObjectOutput
	headErr error
}

func (c *fakeS3PublicationClient) PutObject(_ context.Context, input *awss3.PutObjectInput, _ ...func(*awss3.Options)) (*awss3.PutObjectOutput, error) {
	c.put = input
	return &awss3.PutObjectOutput{}, nil
}

func (c *fakeS3PublicationClient) HeadObject(_ context.Context, _ *awss3.HeadObjectInput, _ ...func(*awss3.Options)) (*awss3.HeadObjectOutput, error) {
	return c.head, c.headErr
}
