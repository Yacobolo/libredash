package mapasset

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

type S3PublicationClient interface {
	PutObject(context.Context, *awss3.PutObjectInput, ...func(*awss3.Options)) (*awss3.PutObjectOutput, error)
	HeadObject(context.Context, *awss3.HeadObjectInput, ...func(*awss3.Options)) (*awss3.HeadObjectOutput, error)
}

type S3PublicationConfig struct {
	Bucket string
	Prefix string
}

type S3PublicationStore struct {
	client S3PublicationClient
	bucket string
	prefix string
}

func NewS3PublicationStore(client S3PublicationClient, config S3PublicationConfig) (*S3PublicationStore, error) {
	if client == nil {
		return nil, fmt.Errorf("S3 publication client is required")
	}
	bucket := strings.TrimSpace(config.Bucket)
	if bucket == "" || strings.ContainsAny(bucket, "/\\\x00\r\n") {
		return nil, fmt.Errorf("S3 publication bucket is invalid")
	}
	prefix := strings.Trim(config.Prefix, "/")
	if prefix != "" && !safePublicationKey(prefix) || strings.ContainsAny(prefix, "\x00\r\n") {
		return nil, fmt.Errorf("S3 publication prefix is invalid")
	}
	return &S3PublicationStore{client: client, bucket: bucket, prefix: prefix}, nil
}

func (s *S3PublicationStore) Stat(ctx context.Context, key string) (PublicationObject, error) {
	objectKey, err := s.objectKey(key)
	if err != nil {
		return PublicationObject{}, err
	}
	output, err := s.client.HeadObject(ctx, &awss3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(objectKey)})
	if err != nil {
		var apiError smithy.APIError
		if errors.As(err, &apiError) && (apiError.ErrorCode() == "NotFound" || apiError.ErrorCode() == "NoSuchKey") {
			return PublicationObject{}, ErrPublicationObjectNotFound
		}
		return PublicationObject{}, err
	}
	if output == nil || output.ContentLength == nil {
		return PublicationObject{}, fmt.Errorf("S3 publication object %q has incomplete metadata", objectKey)
	}
	return PublicationObject{Key: key, Digest: output.Metadata["sha256"], Size: *output.ContentLength, ContentType: aws.ToString(output.ContentType), CacheControl: aws.ToString(output.CacheControl)}, nil
}

func (s *S3PublicationStore) Put(ctx context.Context, object PublicationObject, body io.Reader) error {
	if body == nil || object.Size < 0 || len(object.Digest) != 64 {
		return fmt.Errorf("S3 publication object metadata is invalid")
	}
	checksum, err := hex.DecodeString(object.Digest)
	if err != nil {
		return fmt.Errorf("S3 publication digest is invalid: %w", err)
	}
	objectKey, err := s.objectKey(object.Key)
	if err != nil {
		return err
	}
	_, err = s.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket:               aws.String(s.bucket),
		Key:                  aws.String(objectKey),
		Body:                 body,
		ContentLength:        aws.Int64(object.Size),
		ContentType:          aws.String(object.ContentType),
		CacheControl:         aws.String(object.CacheControl),
		ChecksumSHA256:       aws.String(base64.StdEncoding.EncodeToString(checksum)),
		IfNoneMatch:          aws.String("*"),
		Metadata:             map[string]string{"sha256": object.Digest},
		ServerSideEncryption: types.ServerSideEncryptionAes256,
	})
	if err != nil {
		var apiError smithy.APIError
		if errors.As(err, &apiError) && (apiError.ErrorCode() == "PreconditionFailed" || apiError.ErrorCode() == "ConditionalRequestConflict") {
			return ErrPublicationConflict
		}
	}
	return err
}

func (s *S3PublicationStore) objectKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if !safePublicationKey(key) {
		return "", fmt.Errorf("S3 publication key is invalid")
	}
	if s.prefix == "" {
		return key, nil
	}
	return s.prefix + "/" + key, nil
}

func safePublicationKey(value string) bool {
	return value != "" && !strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "../") && value != ".." && !strings.Contains(value, "/../") && path.Clean(value) == value && value != "." && !strings.ContainsAny(value, "\\\x00\r\n")
}
