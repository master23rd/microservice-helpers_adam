package connections

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	helperconfig "github.com/radian-solusi/microservice-helpers/config"
)

// gcsEndpoint is the Cloud Storage S3-compatible XML API endpoint.
// https://cloud.google.com/storage/docs/aws-simple-migration
const gcsEndpoint = "https://storage.googleapis.com"

type s3Wrapper struct {
	client   *s3.Client
	bucket   string
	provider helperconfig.S3Provider
	root     string // local storage root
	pathURL  string
	mu       sync.RWMutex
}

func safeLocalPath(root, key string) (string, error) {
	clean := filepath.Clean(key)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid object key")
	}
	return filepath.Join(root, clean), nil
}

func NewS3Client(ctx context.Context, cfg helperconfig.S3Config) (S3Client, error) {
	if cfg.Provider == helperconfig.S3ProviderLocal {
		root := cfg.LocalPath
		if root == "" {
			root = "./uploads/users"
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			return nil, fmt.Errorf("create local storage directory: %w", err)
		}
		return &s3Wrapper{provider: helperconfig.S3ProviderLocal, root: root, bucket: cfg.BucketName, pathURL: "/files"}, nil
	}

	region := cfg.Region
	endpoint := cfg.Endpoint
	if cfg.Provider == helperconfig.S3ProviderGCS {
		if region == "" {
			region = "auto"
		}
		if endpoint == "" {
			endpoint = gcsEndpoint
		}
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	w := &s3Wrapper{bucket: cfg.BucketName, provider: cfg.Provider, pathURL: "/images"}

	if endpoint != "" {
		w.client = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
			if cfg.Provider == helperconfig.S3ProviderGCS {
				// GCS XML API does not support the SDK's optional flexible checksums.
				o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
				o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
				o.APIOptions = append(o.APIOptions, w.removeGCSAcceptEncoding)
			}
		})
	} else {
		w.client = s3.NewFromConfig(awsCfg)
	}
	return w, nil
}

// removeGCSAcceptEncoding strips Accept-Encoding before SigV4 signing; GCS's XML API
// rejects requests that include it among the signed headers.
func (s *s3Wrapper) removeGCSAcceptEncoding(stack *middleware.Stack) error {
	if _, ok := stack.Finalize.Get("Signing"); !ok {
		return nil
	}
	return stack.Finalize.Insert(middleware.FinalizeMiddlewareFunc(
		"GCSRemoveAcceptEncoding",
		func(ctx context.Context, input middleware.FinalizeInput, next middleware.FinalizeHandler) (
			middleware.FinalizeOutput, middleware.Metadata, error,
		) {
			if request, ok := input.Request.(*smithyhttp.Request); ok {
				request.Header.Del("Accept-Encoding")
			}
			return next.HandleFinalize(ctx, input)
		},
	), "Signing", middleware.Before)
}

func (s *s3Wrapper) Client() *s3.Client   { return s.client }
func (s *s3Wrapper) IsLocalStorage() bool { return s.provider == helperconfig.S3ProviderLocal }
func (s *s3Wrapper) IsConnected() bool {
	if s.IsLocalStorage() {
		_, err := os.Stat(s.root)
		return err == nil
	}
	if s.client == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.ListFiles(ctx, "/")
	return err == nil
}
func (s *s3Wrapper) Close() error { s.client = nil; return nil }

func (s *s3Wrapper) SetPathURL(path string) { s.mu.Lock(); s.pathURL = path; s.mu.Unlock() }
func (s *s3Wrapper) GetPathURL() string     { s.mu.RLock(); defer s.mu.RUnlock(); return s.pathURL }

func (s *s3Wrapper) UploadFile(ctx context.Context, key string, data []byte, contentType string) error {
	if s.IsLocalStorage() {
		p, err := safeLocalPath(s.root, key)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}
		return os.WriteFile(p, data, 0o644)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
		Body: bytes.NewReader(data), ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("upload to S3: %w", err)
	}
	return nil
}

func (s *s3Wrapper) DownloadFile(ctx context.Context, key string) ([]byte, error) {
	if s.IsLocalStorage() {
		p, err := safeLocalPath(s.root, key)
		if err != nil {
			return nil, err
		}
		return os.ReadFile(p)
	}
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("download from S3: %w", err)
	}
	defer result.Body.Close()
	return io.ReadAll(result.Body)
}

func (s *s3Wrapper) DeleteFile(ctx context.Context, key string) error {
	if s.IsLocalStorage() {
		p, err := safeLocalPath(s.root, key)
		if err != nil {
			return err
		}
		return os.Remove(p)
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete from S3: %w", err)
	}
	return nil
}

func (s *s3Wrapper) ListFiles(ctx context.Context, prefix string) ([]string, error) {
	if s.IsLocalStorage() {
		searchPath := s.root
		if prefix != "" {
			searchPath = filepath.Join(s.root, prefix)
		}
		var keys []string
		err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				rel, _ := filepath.Rel(s.root, path)
				keys = append(keys, filepath.ToSlash(rel))
			}
			return nil
		})
		return keys, err
	}
	input := &s3.ListObjectsV2Input{Bucket: aws.String(s.bucket)}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}
	result, err := s.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("list from S3: %w", err)
	}
	var keys []string
	for _, obj := range result.Contents {
		if obj.Key != nil {
			keys = append(keys, *obj.Key)
		}
	}
	return keys, nil
}

func (s *s3Wrapper) FileExists(ctx context.Context, key string) (bool, error) {
	if s.IsLocalStorage() {
		p, err := safeLocalPath(s.root, key)
		if err != nil {
			return false, err
		}
		_, err = os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	})
	if err != nil {
		var nfe *s3types.NotFound
		if errors.As(err, &nfe) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *s3Wrapper) GetFileURL(ctx context.Context, key string, expiration time.Duration) (string, error) {
	if s.IsLocalStorage() {
		host := os.Getenv("HOST")
		return host + s.GetPathURL() + "?to=" + key, nil
	}
	presignClient := s3.NewPresignClient(s.client)
	result, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	}, func(opts *s3.PresignOptions) { opts.Expires = expiration })
	if err != nil {
		return "", fmt.Errorf("presign URL: %w", err)
	}
	return result.URL, nil
}

func (s *s3Wrapper) GetFileExtension(key string) string { return filepath.Ext(key) }
