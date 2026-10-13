package connections

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	client          *s3.Client
	bucket          string
	provider        helperconfig.S3Provider
	root            string // local storage root
	pathURL         string
	accessKeyID     string // HMAC key id, needed for GCS native V4 signing
	secretAccessKey string // HMAC secret, needed for GCS native V4 signing
	mu              sync.RWMutex
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

	w := &s3Wrapper{
		bucket:          cfg.BucketName,
		provider:        cfg.Provider,
		pathURL:         "/images",
		accessKeyID:     cfg.AccessKeyID,
		secretAccessKey: cfg.SecretAccessKey,
	}

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
	if s.provider == helperconfig.S3ProviderGCS {
		return s.signGCSURLV4(key, expiration)
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

// gcsMinExpiry and gcsMaxExpiry are GCS's accepted bounds for V4 signed URLs.
// https://cloud.google.com/storage/docs/access-control/signed-urls-v4
const (
	gcsMinExpiry = 1 * time.Second
	gcsMaxExpiry = 604800 * time.Second
)

// signGCSURLV4 builds a GCS V4 signed URL using HMAC credentials (access key
// id / secret), without requiring a service-account JSON key. The AWS SDK's
// presigner cannot be reused here: GCS's XML API rejects the SigV4-style
// epoch "Expires" param it produces, requiring the duration-based
// "X-Goog-Expires" param instead. See:
// https://cloud.google.com/storage/docs/access-control/signed-urls-v4
func (s *s3Wrapper) signGCSURLV4(key string, expiration time.Duration) (string, error) {
	if expiration < gcsMinExpiry {
		expiration = gcsMinExpiry
	} else if expiration > gcsMaxExpiry {
		expiration = gcsMaxExpiry
	}

	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")
	credentialScope := fmt.Sprintf("%s/auto/storage/goog4_request", dateStamp)
	credential := fmt.Sprintf("%s/%s", s.accessKeyID, credentialScope)

	host := "storage.googleapis.com"
	canonicalURI := "/" + s.bucket + "/" + key

	query := url.Values{}
	query.Set("X-Goog-Algorithm", "GOOG4-HMAC-SHA256")
	query.Set("X-Goog-Credential", credential)
	query.Set("X-Goog-Date", amzDate)
	query.Set("X-Goog-Expires", strconv.Itoa(int(expiration.Seconds())))
	query.Set("X-Goog-SignedHeaders", "host")

	canonicalQueryString := encodeCanonicalQuery(query)
	canonicalHeaders := "host:" + host + "\n"
	signedHeaders := "host"
	canonicalRequest := strings.Join([]string{
		"GET",
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		"UNSIGNED-PAYLOAD",
	}, "\n")

	hashedCanonicalRequest := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{
		"GOOG4-HMAC-SHA256",
		amzDate,
		credentialScope,
		hex.EncodeToString(hashedCanonicalRequest[:]),
	}, "\n")

	signingKey := gcsSigningKey(s.secretAccessKey, dateStamp)
	signature := hmac.New(sha256.New, signingKey)
	signature.Write([]byte(stringToSign))
	query.Set("X-Goog-Signature", hex.EncodeToString(signature.Sum(nil)))

	u := url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     canonicalURI,
		RawQuery: encodeCanonicalQuery(query),
	}
	return u.String(), nil
}

// gcsSigningKey derives the GOOG4 signing key, mirroring AWS SigV4's derived
// key chain but with the "GOOG4" prefix and a fixed "storage"/"goog4_request"
// scope, per the GCS V4 signing spec.
func gcsSigningKey(secret, dateStamp string) []byte {
	h := func(key, data []byte) []byte {
		mac := hmac.New(sha256.New, key)
		mac.Write(data)
		return mac.Sum(nil)
	}
	kDate := h([]byte("GOOG4"+secret), []byte(dateStamp))
	kRegion := h(kDate, []byte("auto"))
	kService := h(kRegion, []byte("storage"))
	return h(kService, []byte("goog4_request"))
}

// encodeCanonicalQuery renders query params sorted by key with RFC 3986
// percent-encoding, as required by the canonical request format.
func encodeCanonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(rfc3986Escape(k))
		buf.WriteByte('=')
		buf.WriteString(rfc3986Escape(q.Get(k)))
	}
	return buf.String()
}

// rfc3986Escape percent-encodes like url.QueryEscape but keeps spaces as
// "%20" instead of "+", matching the RFC 3986 encoding GCS/AWS signing
// requires for canonical query strings.
func rfc3986Escape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

func (s *s3Wrapper) GetFileExtension(key string) string { return filepath.Ext(key) }
