package storage

import (
	"context"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rs/zerolog/log"
)

type MinioStorage struct {
	client         *minio.Client
	bucket         string
	internalEndpoint string // e.g. "minio:9000" (Docker internal)
	publicEndpoint   string // e.g. "localhost:9000" (browser-accessible)
}

func NewMinioStorage(endpoint, accessKey, secretKey, bucket string, useSSL bool, publicEndpoint string) *MinioStorage {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create minio client")
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		log.Fatal().Err(err).Str("bucket", bucket).Msg("failed to check bucket existence")
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			log.Fatal().Err(err).Str("bucket", bucket).Msg("failed to create bucket")
		}
		log.Info().Str("bucket", bucket).Msg("minio bucket created")
	}

	if publicEndpoint == "" {
		publicEndpoint = endpoint
	}

	log.Info().Str("endpoint", endpoint).Msg("minio connected")
	return &MinioStorage{
		client:           client,
		bucket:           bucket,
		internalEndpoint: endpoint,
		publicEndpoint:   publicEndpoint,
	}
}

// PutObject uploads a file from a reader. contentType e.g. "application/pdf".
func (s *MinioStorage) PutObject(ctx context.Context, objectKey, contentType string, reader io.Reader, size int64) error {
	_, err := s.client.PutObject(ctx, s.bucket, objectKey, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

// PutFile uploads a local file by path.
func (s *MinioStorage) PutFile(ctx context.Context, objectKey, filePath, contentType string) error {
	_, err := s.client.FPutObject(ctx, s.bucket, objectKey, filePath, minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

// PresignedGetURL returns a time-limited download URL using the public endpoint.
func (s *MinioStorage) PresignedGetURL(ctx context.Context, objectKey string, expiry time.Duration) (*url.URL, error) {
	u, err := s.client.PresignedGetObject(ctx, s.bucket, objectKey, expiry, nil)
	if err != nil {
		return nil, err
	}

	// Rewrite internal Docker hostname to browser-accessible public endpoint.
	if s.internalEndpoint != s.publicEndpoint {
		raw := u.String()
		raw = strings.ReplaceAll(raw, s.internalEndpoint, s.publicEndpoint)
		if rewritten, err := url.Parse(raw); err == nil {
			return rewritten, nil
		}
	}

	return u, nil
}

// RemoveObject deletes an object from MinIO.
func (s *MinioStorage) RemoveObject(ctx context.Context, objectKey string) error {
	return s.client.RemoveObject(ctx, s.bucket, objectKey, minio.RemoveObjectOptions{})
}
