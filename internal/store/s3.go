package store

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Config holds configuration for the S3 snapshot storage backend.
type S3Config struct {
	Endpoint  string
	Bucket    string
	Region    string
	AccessKey string
	SecretKey string
}

// S3Client wraps an S3-compatible client for snapshot storage.
type S3Client struct {
	client *s3.Client
	bucket string
}

// NewS3Client creates a new S3 client from the given config.
func NewS3Client(cfg S3Config) (*S3Client, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("S3 bucket is required")
	}

	opts := []func(*s3.Options){
		func(o *s3.Options) {
			o.Region = cfg.Region
			o.Credentials = credentials.NewStaticCredentialsProvider(
				cfg.AccessKey, cfg.SecretKey, "",
			)
		},
	}

	if cfg.Endpoint != "" {
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true // required for MinIO and most S3-compatible stores
		})
	}

	client := s3.New(s3.Options{}, opts...)

	return &S3Client{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

// Upload writes the contents of reader to the given S3 key.
func (c *S3Client) Upload(ctx context.Context, key string, reader io.Reader) error {
	slog.Info("uploading to S3", "bucket", c.bucket, "key", key)
	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
		Body:   reader,
	})
	if err != nil {
		return fmt.Errorf("s3 upload %s: %w", key, err)
	}
	return nil
}

// UploadFile uploads a local file to S3.
func (c *S3Client) UploadFile(ctx context.Context, key, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file for upload: %w", err)
	}
	defer f.Close()
	return c.Upload(ctx, key, f)
}

// Download fetches an S3 object and writes it to the local dest path.
func (c *S3Client) Download(ctx context.Context, key, dest string) error {
	slog.Info("downloading from S3", "bucket", c.bucket, "key", key, "dest", dest)
	out, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3 download %s: %w", key, err)
	}
	defer out.Body.Close()

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create dest file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, out.Body); err != nil {
		return fmt.Errorf("write dest file: %w", err)
	}
	return nil
}

// UploadFileCompressed gzip-compresses a local file and uploads it to S3.
// Compression is written to a temp file so the AWS SDK can seek the body
// for payload signing and retries.
func (c *S3Client) UploadFileCompressed(ctx context.Context, key, filePath string) error {
	slog.Info("uploading compressed to S3", "bucket", c.bucket, "key", key)

	src, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file for compressed upload: %w", err)
	}
	defer src.Close()

	tmp, err := os.CreateTemp("", "hatch-gz-*")
	if err != nil {
		return fmt.Errorf("create temp file for compressed upload: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	gw := gzip.NewWriter(tmp)
	if _, err := io.Copy(gw, src); err != nil {
		return fmt.Errorf("compress file: %w", err)
	}
	if err := gw.Close(); err != nil {
		return fmt.Errorf("finalize gzip: %w", err)
	}

	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek compressed file: %w", err)
	}

	_, err = c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
		Body:   tmp,
	})
	if err != nil {
		return fmt.Errorf("s3 upload compressed %s: %w", key, err)
	}
	return nil
}

// DownloadCompressed fetches a gzip-compressed S3 object, decompresses it,
// and writes the result to the local dest path.
func (c *S3Client) DownloadCompressed(ctx context.Context, key, dest string) error {
	slog.Info("downloading compressed from S3", "bucket", c.bucket, "key", key, "dest", dest)
	out, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3 download %s: %w", key, err)
	}
	defer out.Body.Close()

	gr, err := gzip.NewReader(out.Body)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gr.Close()

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create dest file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, gr); err != nil {
		return fmt.Errorf("decompress to dest: %w", err)
	}
	return nil
}

// Delete removes an object from S3.
func (c *S3Client) Delete(ctx context.Context, key string) error {
	slog.Info("deleting from S3", "bucket", c.bucket, "key", key)
	_, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3 delete %s: %w", key, err)
	}
	return nil
}
