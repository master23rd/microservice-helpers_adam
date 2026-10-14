package config

type DBType string

const (
	DBTypePostgres DBType = "postgres"
	DBTypeMySQL    DBType = "mysql"
	DBTypeSQLite   DBType = "sqlite"
)

type S3Provider string

const (
	S3ProviderAWS   S3Provider = "aws"
	S3ProviderMinio S3Provider = "minio"
	S3ProviderLocal S3Provider = "local"
	S3ProviderGCS   S3Provider = "gcs"
)

type SftpConfig struct {
	Host           string `toml:"host"`
	Port           int    `toml:"port"`
	User           string `toml:"user"`
	Password       string `toml:"password"`
	KnownHostsFile string `toml:"known_hosts_file"`
	HostKey        string `toml:"host_key"`
}

type GPubSubConfig struct {
	ProjectID       string            `toml:"project_id"`
	Topics          map[string]string `toml:"topics"`
	SubscriptionID  string            `toml:"subscription_id"`
	CredentialsFile string            `toml:"credentials_file"`
	EmulatorHost    string            `toml:"emulator_host"`
}

type RedisConfig struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	Password string `toml:"password"`
	DB       int    `toml:"db"`
	DBPubSub int    `toml:"db_pub_sub"`
}

type CorsConfig struct {
	AllowedUrl string `toml:"allowed_url"`
	RateLimit  int    `toml:"rate_limit"`
}

type DatabaseConfig struct {
	Type            DBType `toml:"type"`
	Host            string `toml:"host"`
	Port            int    `toml:"port"`
	User            string `toml:"user"`
	Password        string `toml:"password"`
	DBName          string `toml:"db_name"`
	MaxOpenConns    int    `toml:"max_open_conns"`
	MaxIdleConns    int    `toml:"max_idle_conns"`
	ConnMaxLifetime int    `toml:"conn_max_lifetime_minutes"`
	ConnMaxIdleTime int    `toml:"conn_max_idle_time_minutes"`
}

type S3Config struct {
	Provider        S3Provider `toml:"provider"`
	Endpoint        string     `toml:"endpoint"`
	Region          string     `toml:"region"`
	AccessKeyID     string     `toml:"access_key_id"`
	SecretAccessKey string     `toml:"secret_access_key"`
	UseSSL          bool       `toml:"use_ssl"`
	BucketName      string     `toml:"bucket_name"`
	LocalPath       string     `toml:"local_path"`
}

// DefaultMaxUploadSizeMB is the request body size limit used when
// AppConfig.MaxUploadSizeMB is unset (0 or negative).
const DefaultMaxUploadSizeMB = 50

type AppConfig struct {
	AppKey string `toml:"app_key"`
	// LegacyAppKey is the e-IPO (PHP) secretKey used for data encrypted outside
	// the Go stack. Kept separate from AppKey, which signs tokens and must not change.
	LegacyAppKey     string `toml:"legacy_app_key"`
	LimitData        int    `toml:"limit_data"`
	SessionMode      string `toml:"session_mode"`
	OtpEnabled       bool   `toml:"otp_enabled"`
	SmsOtpEnabled    bool   `toml:"sms_otp_enabled"`
	OtpIssuer        string `toml:"otp_issuer"`
	OtpToken         string `toml:"otp_token"`
	ValidateOverKsei bool   `toml:"validate_over_ksei"`
	MFAUserTypes     []int  `toml:"mfa_user_types"`
	FrontendURL      string `toml:"frontend_url"`
	// MaxUploadSizeMB caps the request body size (http.MaxBytesReader) in MB.
	// 0 or unset falls back to DefaultMaxUploadSizeMB via MaxUploadBytes.
	MaxUploadSizeMB int `toml:"max_upload_size_mb"`
}

// MaxUploadBytes returns the configured max request body size in bytes,
// defaulting to DefaultMaxUploadSizeMB when MaxUploadSizeMB is unset or <= 0.
func (c AppConfig) MaxUploadBytes() int64 {
	mb := c.MaxUploadSizeMB
	if mb <= 0 {
		mb = DefaultMaxUploadSizeMB
	}
	return int64(mb) * 1024 * 1024
}

type MongoDBConfig struct {
	URI      string `toml:"uri"`
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	User     string `toml:"user"`
	Password string `toml:"password"`
	DBName   string `toml:"db_name"`
}

type OtelConfig struct {
	Enabled        bool   `toml:"enabled"`
	ServiceName    string `toml:"service_name"`
	ServiceVersion string `toml:"service_version"`
	GCPProjectID   string `toml:"gcp_project_id"`
}

type RecaptchaConfig struct {
	Enabled    bool   `toml:"enabled"`
	Enterprise bool   `toml:"enterprise"`
	SecretKey  string `toml:"secret_key"`
}

type MainConfig struct {
	Sftp      SftpConfig      `toml:"sftp"`
	GPubSub   GPubSubConfig   `toml:"google_pubsub"`
	Redis     RedisConfig     `toml:"redis"`
	Cors      CorsConfig      `toml:"cors"`
	Database  DatabaseConfig  `toml:"database"`
	S3        S3Config        `toml:"s3"`
	App       AppConfig       `toml:"app"`
	MongoDB   MongoDBConfig   `toml:"mongodb"`
	Otel      OtelConfig      `toml:"opentelemetry"`
	Recaptcha RecaptchaConfig `toml:"recaptcha"`
}
