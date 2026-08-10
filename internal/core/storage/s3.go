// This file implements the S3-compatible API backend of the storage
// port: any S3-compatible endpoint (MinIO, Garage, Ceph, R2, ...) is
// supported, AWS being just another endpoint. The client is configured
// with an endpoint, credentials and region of the operator's choosing.
// The bucket is expected to exist; Verify checks it at startup and
// surfaces a clear error.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config selects and authenticates the S3-compatible endpoint.
type S3Config struct {
	// Endpoint is the API base, e.g. "minio.example.org:9000".
	Endpoint string
	// Bucket stores every object.
	Bucket string
	// AccessKey and SecretKey are the API credentials.
	AccessKey string
	SecretKey string
	// Region is used for signature calculation; may be empty for
	// non-AWS endpoints.
	Region string
	// Secure selects HTTPS. Development MinIO instances are usually
	// served over plain HTTP.
	Secure bool
	// PathStyle forces path-style addressing (bucket in the path).
	// Path-style is the default for S3-compatible servers; AWS
	// virtual-host style needs this disabled.
	PathStyle bool
}

// S3 is the S3-compatible API implementation of Store.
type S3 struct {
	client *minio.Client
	bucket string
}

// NewS3 builds the client from cfg without touching the network.
func NewS3(cfg S3Config) (*S3, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("storage/s3: endpoint is required")
	}
	if cfg.Bucket == "" {
		return nil, errors.New("storage/s3: bucket is required")
	}
	lookup := minio.BucketLookupAuto
	if cfg.PathStyle {
		lookup = minio.BucketLookupPath
	}
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:       cfg.Secure,
		Region:       cfg.Region,
		BucketLookup: lookup,
	})
	if err != nil {
		return nil, fmt.Errorf("storage/s3: client: %w", err)
	}
	return &S3{client: client, bucket: cfg.Bucket}, nil
}

// Verify reports whether the bucket is reachable and exists. It is
// called at startup so a misconfigured backend fails fast.
func (s *S3) Verify(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("storage/s3: bucket check %q: %w", s.bucket, err)
	}
	if !exists {
		return fmt.Errorf("storage/s3: bucket %q does not exist", s.bucket)
	}
	return nil
}

// Put stores data at key, overwriting any previous object.
func (s *S3) Put(ctx context.Context, key string, data io.Reader, size int64, contentType string) (ObjectInfo, error) {
	if err := ValidKey(key); err != nil {
		return ObjectInfo{}, err
	}
	info, err := s.client.PutObject(ctx, s.bucket, key, data, size,
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("storage/s3: put %q: %w", key, err)
	}
	return ObjectInfo{
		Key:          key,
		Size:         info.Size,
		ContentType:  contentType,
		ETag:         info.ETag,
		LastModified: info.LastModified,
	}, nil
}

// Get returns the object contents and metadata.
func (s *S3) Get(ctx context.Context, key string) (*Object, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, mapErr(key, err)
	}
	info, err := obj.Stat()
	if err != nil {
		obj.Close()
		return nil, mapErr(key, err)
	}
	return &Object{
		ObjectInfo: ObjectInfo{
			Key:          key,
			Size:         info.Size,
			ContentType:  info.ContentType,
			ETag:         info.ETag,
			LastModified: info.LastModified,
		},
		Data: obj,
	}, nil
}

// Delete removes the object; missing objects are not an error.
func (s *S3) Delete(ctx context.Context, key string) error {
	if err := ValidKey(key); err != nil {
		return err
	}
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("storage/s3: delete %q: %w", key, err)
	}
	return nil
}

// Stat returns the object metadata without its contents.
func (s *S3) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return ObjectInfo{}, mapErr(key, err)
	}
	return ObjectInfo{
		Key:          key,
		Size:         info.Size,
		ContentType:  info.ContentType,
		ETag:         info.ETag,
		LastModified: info.LastModified,
	}, nil
}

// List returns the objects under prefix, sorted by key.
func (s *S3) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	var out []ObjectInfo
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("storage/s3: list %q: %w", prefix, obj.Err)
		}
		out = append(out, ObjectInfo{
			Key:          obj.Key,
			Size:         obj.Size,
			ContentType:  obj.ContentType,
			ETag:         obj.ETag,
			LastModified: obj.LastModified,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// mapErr converts S3 API errors into storage errors.
func mapErr(key string, err error) error {
	var resp minio.ErrorResponse
	if errors.As(err, &resp) && resp.Code == "NoSuchKey" {
		return ErrNotFound
	}
	return fmt.Errorf("storage/s3: %q: %w", key, err)
}
