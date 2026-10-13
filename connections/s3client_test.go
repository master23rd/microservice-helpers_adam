package connections

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	helperconfig "github.com/radian-solusi/microservice-helpers/config"
)

func TestGetFileURLGCSUsesNativeV4Signing(t *testing.T) {
	c, err := NewS3Client(context.Background(), helperconfig.S3Config{
		Provider:        helperconfig.S3ProviderGCS,
		BucketName:      "my-bucket",
		AccessKeyID:     "GOOGACCESSKEY",
		SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	got, err := c.GetFileURL(context.Background(), "docs/report.pdf", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("GetFileURL returned invalid URL: %v", err)
	}
	q := u.Query()
	if q.Get("X-Goog-Expires") != "3600" {
		t.Fatalf("X-Goog-Expires = %q, want 3600", q.Get("X-Goog-Expires"))
	}
	if q.Get("X-Goog-Signature") == "" {
		t.Fatal("X-Goog-Signature must be set")
	}
	if q.Get("X-Goog-Algorithm") != "GOOG4-HMAC-SHA256" {
		t.Fatalf("X-Goog-Algorithm = %q, want GOOG4-HMAC-SHA256", q.Get("X-Goog-Algorithm"))
	}
	if strings.Contains(got, "Expires=") && !strings.Contains(got, "X-Goog-Expires=") {
		t.Fatalf("URL must not contain AWS-style Expires param: %s", got)
	}
	if q.Get("Expires") != "" {
		t.Fatalf("URL must not contain AWS-style epoch Expires param, got %s", q.Get("Expires"))
	}
}

func TestGetFileURLGCSClampsExpiration(t *testing.T) {
	c, err := NewS3Client(context.Background(), helperconfig.S3Config{
		Provider:        helperconfig.S3ProviderGCS,
		BucketName:      "my-bucket",
		AccessKeyID:     "GOOGACCESSKEY",
		SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	cases := []struct {
		name       string
		expiration time.Duration
		want       string
	}{
		{"zero clamps to 1s", 0, "1"},
		{"negative clamps to 1s", -5 * time.Second, "1"},
		{"over max clamps to 604800s", 999999 * time.Second, "604800"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := c.GetFileURL(context.Background(), "key", tc.expiration)
			if err != nil {
				t.Fatal(err)
			}
			u, err := url.Parse(got)
			if err != nil {
				t.Fatal(err)
			}
			if gotExp := u.Query().Get("X-Goog-Expires"); gotExp != tc.want {
				t.Fatalf("X-Goog-Expires = %q, want %q", gotExp, tc.want)
			}
		})
	}
}

func TestGetFileURLGCSSignsWithCustomEndpointHost(t *testing.T) {
	c, err := NewS3Client(context.Background(), helperconfig.S3Config{
		Provider:        helperconfig.S3ProviderGCS,
		BucketName:      "my-bucket",
		AccessKeyID:     "GOOGACCESSKEY",
		SecretAccessKey: "secret",
		Endpoint:        "http://localhost:4443",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	got, err := c.GetFileURL(context.Background(), "docs/report.pdf", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("GetFileURL returned invalid URL: %v", err)
	}
	if u.Scheme != "http" || u.Host != "localhost:4443" {
		t.Fatalf("signed URL host = %s://%s, want http://localhost:4443 (must follow configured endpoint, not a hardcoded host)", u.Scheme, u.Host)
	}
}

func TestGetFileURLGCSDefaultEndpointStillUsesGoogleHost(t *testing.T) {
	c, err := NewS3Client(context.Background(), helperconfig.S3Config{
		Provider:        helperconfig.S3ProviderGCS,
		BucketName:      "my-bucket",
		AccessKeyID:     "GOOGACCESSKEY",
		SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	got, err := c.GetFileURL(context.Background(), "docs/report.pdf", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "storage.googleapis.com" {
		t.Fatalf("signed URL host = %s, want storage.googleapis.com when endpoint is unset", u.Host)
	}
}

func TestGetFileURLS3StillUsesAWSPresign(t *testing.T) {
	c, err := NewS3Client(context.Background(), helperconfig.S3Config{
		Provider:        helperconfig.S3ProviderAWS,
		BucketName:      "my-bucket",
		Region:          "us-east-1",
		AccessKeyID:     "AKIA",
		SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	got, err := c.GetFileURL(context.Background(), "key", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("X-Amz-Expires") != "3600" {
		t.Fatalf("X-Amz-Expires = %q, want 3600 (regression: s3 provider must keep AWS presign)", u.Query().Get("X-Amz-Expires"))
	}
	if u.Query().Get("X-Goog-Signature") != "" {
		t.Fatal("s3 provider must not produce GCS-style signature params")
	}
}

func TestGCSProviderDefaultsToXMLEndpoint(t *testing.T) {
	c, err := NewS3Client(context.Background(), helperconfig.S3Config{
		Provider:        helperconfig.S3ProviderGCS,
		BucketName:      "my-bucket",
		AccessKeyID:     "GOOGACCESSKEY",
		SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if c.IsLocalStorage() {
		t.Fatal("GCS client must not report as local storage")
	}
	client := c.Client()
	if client == nil {
		t.Fatal("GCS provider must use the AWS SDK *s3.Client (HMAC via S3 XML API)")
	}
	opts := client.Options()
	if opts.BaseEndpoint == nil || *opts.BaseEndpoint != gcsEndpoint {
		t.Fatalf("BaseEndpoint = %v, want %s", opts.BaseEndpoint, gcsEndpoint)
	}
	if !opts.UsePathStyle {
		t.Fatal("expected path-style addressing for GCS")
	}
	if opts.Region != "auto" {
		t.Fatalf("Region = %q, want auto", opts.Region)
	}
	if opts.RequestChecksumCalculation != aws.RequestChecksumCalculationWhenRequired {
		t.Fatalf("RequestChecksumCalculation = %v, want WhenRequired", opts.RequestChecksumCalculation)
	}
	if opts.ResponseChecksumValidation != aws.ResponseChecksumValidationWhenRequired {
		t.Fatalf("ResponseChecksumValidation = %v, want WhenRequired", opts.ResponseChecksumValidation)
	}
}

func TestGCSProviderHonorsExplicitEndpointAndRegion(t *testing.T) {
	c, err := NewS3Client(context.Background(), helperconfig.S3Config{
		Provider:        helperconfig.S3ProviderGCS,
		BucketName:      "my-bucket",
		Endpoint:        "http://127.0.0.1:4443", // e.g. fake-gcs-server
		Region:          "us-east-1",
		AccessKeyID:     "GOOGACCESSKEY",
		SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	opts := c.Client().Options()
	if opts.BaseEndpoint == nil || *opts.BaseEndpoint != "http://127.0.0.1:4443" {
		t.Fatalf("BaseEndpoint = %v, want explicit override", opts.BaseEndpoint)
	}
	if opts.Region != "us-east-1" {
		t.Fatalf("Region = %q, want explicit override", opts.Region)
	}
}

func TestGCSGetFileExtension(t *testing.T) {
	c, err := NewS3Client(context.Background(), helperconfig.S3Config{
		Provider:        helperconfig.S3ProviderGCS,
		BucketName:      "my-bucket",
		AccessKeyID:     "GOOGACCESSKEY",
		SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if got := c.GetFileExtension("docs/report.PDF"); got != ".PDF" {
		t.Fatalf("extension: got %q", got)
	}
}

func TestAWSProviderUnaffectedByGCSDefaults(t *testing.T) {
	c, err := NewS3Client(context.Background(), helperconfig.S3Config{
		Provider:        helperconfig.S3ProviderAWS,
		BucketName:      "my-bucket",
		Region:          "ap-southeast-1",
		AccessKeyID:     "AKIA",
		SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	opts := c.Client().Options()
	if opts.BaseEndpoint != nil {
		t.Fatalf("BaseEndpoint = %v, want nil for default AWS endpoint resolution", opts.BaseEndpoint)
	}
	if opts.Region != "ap-southeast-1" {
		t.Fatalf("Region = %q, want ap-southeast-1", opts.Region)
	}
	if opts.RequestChecksumCalculation == aws.RequestChecksumCalculationWhenRequired {
		t.Fatal("AWS provider must keep SDK default checksum behavior, not the GCS override")
	}
}

func TestMinioProviderUsesPathStyleWithoutGCSDefaults(t *testing.T) {
	c, err := NewS3Client(context.Background(), helperconfig.S3Config{
		Provider:        helperconfig.S3ProviderMinio,
		BucketName:      "my-bucket",
		Endpoint:        "http://127.0.0.1:9000",
		Region:          "us-east-1",
		AccessKeyID:     "minioadmin",
		SecretAccessKey: "minioadmin",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	opts := c.Client().Options()
	if opts.BaseEndpoint == nil || *opts.BaseEndpoint != "http://127.0.0.1:9000" {
		t.Fatalf("BaseEndpoint = %v, want minio endpoint", opts.BaseEndpoint)
	}
	if !opts.UsePathStyle {
		t.Fatal("expected path-style addressing for minio")
	}
	if opts.RequestChecksumCalculation == aws.RequestChecksumCalculationWhenRequired {
		t.Fatal("minio provider must keep SDK default checksum behavior, not the GCS override")
	}
}
