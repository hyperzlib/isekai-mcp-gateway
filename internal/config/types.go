package config

import "encoding/json"

type Server struct {
	Host                string    `json:"host"`
	Port                int       `json:"port"`
	PublicURL           string    `json:"publicUrl"`
	AllowedHosts        []string  `json:"allowedHosts"`
	AllowedOrigins      []string  `json:"allowedOrigins"`
	RequestBodyMaxBytes int64     `json:"requestBodyMaxBytes"`
	TLS                 ServerTLS `json:"tls"`
}

// ServerTLS enables HTTPS on the same listener as MCP and OAuth.
type ServerTLS struct {
	Enabled  bool   `json:"enabled"`
	CertFile string `json:"certFile"`
	KeyFile  string `json:"keyFile"`
}

type Client struct {
	URI            string   `json:"client_uri,omitempty"`
	LogoURL        *string  `json:"logo_uri,omitempty"`
	ID             string   `json:"client_id"`
	Name           string   `json:"client_name"`
	Redirects      []string `json:"redirect_uris"`
	AuthMethod     string   `json:"token_endpoint_auth_method"`
	SecretEnv      string   `json:"clientSecretEnv,omitempty"`
	Secret         string   `json:"clientSecret,omitempty" secret:"true"`
	Scope          string   `json:"scope"`
	Grants         []string `json:"grant_types"`
	Responses      []string `json:"response_types"`
	ResourceClient bool     `json:"resourceClient,omitempty"`
}

type Gateway struct {
	Issuer            string       `json:"issuer"`
	Resource          string       `json:"resource"`
	AccessTokenFormat string       `json:"accessTokenFormat"`
	CodeTTL           int          `json:"authorizationCodeTtlSeconds"`
	AccessTTL         int          `json:"accessTokenTtlSeconds"`
	InteractionTTL    int          `json:"interactionTtlSeconds"`
	RefreshTTL        int          `json:"refreshTokenIdleTtlSeconds"`
	GrantTTL          int          `json:"grantMaxTtlSeconds"`
	CookieSecretEnv   string       `json:"cookieSecretEnv"`
	CookieSecret      string       `json:"cookieSecret,omitempty" secret:"true"`
	StorageKeyEnv     string       `json:"storageEncryptionKeyEnv"`
	StorageKey        string       `json:"storageEncryptionKey,omitempty" secret:"true"`
	SigningEnv        string       `json:"signingJwksEnv"`
	SigningJWKS       *SigningJWKS `json:"signingJwks,omitempty" secret:"true"`
	Registration      struct {
		CIMD       bool     `json:"cimd"`
		Dynamic    bool     `json:"dynamic"`
		Clients    []Client `json:"preRegisteredClients"`
		MaxClients int      `json:"maxClients"`
		IdleTTL    int      `json:"idleClientTtlSeconds"`
	} `json:"registration"`
	Fetch struct {
		TimeoutMS    int   `json:"timeoutMs"`
		MaxBytes     int64 `json:"maxBytes"`
		MaxRedirects int   `json:"maxRedirects"`
	} `json:"metadataFetch"`
}

type Storage struct {
	Driver      string `json:"driver"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Database    string `json:"database"`
	Username    string `json:"username"`
	PasswordEnv string `json:"passwordEnv"`
	Password    string `json:"password,omitempty" secret:"true"`
	PoolSize    int    `json:"poolSize"`
	TLS         struct {
		Enabled    bool   `json:"enabled"`
		CAPath     string `json:"caPath,omitempty"`
		ServerName string `json:"serverName,omitempty"`
	} `json:"tls"`
}

type ServerFile struct {
	Schema  string `json:"$schema,omitempty"`
	Version int    `json:"configVersion"`
	Server  Server `json:"server"`
	Auth    struct {
		Gateway Gateway `json:"gateway"`
	} `json:"auth"`
	Storage    Storage `json:"storage"`
	Extensions struct {
		ACL struct {
			Enabled bool `json:"enabled"`
		} `json:"IsekaiLitePageACL"`
		Moderation struct {
			Enabled bool `json:"enabled"`
		} `json:"Moderation"`
	} `json:"extensions"`
}

type OidcConnect struct {
	Issuer      string   `json:"issuer"`
	ClientID    string   `json:"clientId"`
	SecretEnv   string   `json:"clientSecretEnv"`
	Secret      string   `json:"clientSecret,omitempty" secret:"true"`
	RedirectURI string   `json:"redirectUri"`
	Scopes      []string `json:"authorizationScopes"`
}

type MediaWiki struct {
	SiteID       string `json:"siteId"`
	APIURL       string `json:"apiUrl"`
	TimeoutMS    int    `json:"timeoutMs"`
	Maxlag       int    `json:"maxlagSeconds"`
	PageCacheTTL int    `json:"pageCacheTtlSeconds"`
	Bridge       struct {
		KeyID     string `json:"keyId"`
		SecretEnv string `json:"secretEnv"`
		Secret    string `json:"secret,omitempty" secret:"true"`
	} `json:"serviceBridge"`
}

// Future business configuration remains parseable but does not register tools.
type AppFile struct {
	Site    Site   `json:"site"`
	Schema  string `json:"$schema,omitempty"`
	Version int    `json:"configVersion"`
	Auth    struct {
		OidcClient OidcConnect `json:"oidc_client"`
		Consent    struct {
			DefaultSelectedScopes []string `json:"defaultSelectedScopes"`
		} `json:"consent"`
	} `json:"auth"`
	MediaWiki MediaWiki `json:"mediawiki"`
	Editing   struct {
		TTL           int   `json:"sessionTtlSeconds"`
		MaxActive     int   `json:"maxActiveSessionsPerUser"`
		MaxContent    int64 `json:"maxContentBytes"`
		MaxOperations int   `json:"maxOperations"`
	} `json:"editing"`
	Uploads struct {
		TaskTTL           int      `json:"uploadTaskTtlSeconds"`
		Retention         int      `json:"resultRetentionSeconds"`
		MaxBytes          int64    `json:"maxFileBytes"`
		MaxActive         int      `json:"maxActiveTicketsPerPrincipal"`
		ReservedPrincipal int64    `json:"maxReservedBytesPerPrincipal"`
		ReservedGlobal    int64    `json:"maxReservedBytesGlobal"`
		TempPath          string   `json:"tempPath"`
		Mimes             []string `json:"allowedMimeTypes"`
		StagedTTL         int      `json:"stagedFileTtlSeconds"`
	} `json:"uploads"`
	Results struct {
		TextMax    int64 `json:"defaultTextMaxBytes"`
		PreviewMax int64 `json:"maxPreviewBytes"`
	} `json:"results"`
	Additions struct {
		Foreign struct {
			Exemption  string   `json:"exemption"`
			Operations []string `json:"operations"`
			TTL        int      `json:"challengeTtlSeconds"`
			Version    int      `json:"policyVersion"`
		} `json:"foreignPageConfirmation"`
		Tags struct {
			Mapping map[string]string `json:"mapping"`
		} `json:"aiRevisionTags"`
	} `json:"additions"`
	Logging Logging `json:"logging"`
}

type Logging struct {
	TraceRequest  bool   `json:"traceRequest"`
	Level         string `json:"level"`
	ConsoleFormat string `json:"consoleFormat"`
	File          struct {
		Enabled   bool   `json:"enabled"`
		Split     bool   `json:"split"`
		Path      string `json:"path"`
		ErrorPath string `json:"errorPath"`
	} `json:"file"`
}

type Secrets struct {
	Cookie           []byte
	Storage          []byte
	Signing          string
	DBPassword       string
	OidcClientSecret string
	BridgeSecret     string
	ClientSecrets    map[string]string
}

// Config is immutable after publication. Readers take one snapshot per operation.
type Config struct {
	Static  ServerFile
	App     AppFile
	Secrets Secrets
	Dir     string
}

// SigningJWKS keeps private JWK attributes opaque until cryptographic validation.
type SigningJWKS struct {
	Keys []json.RawMessage `json:"keys"`
}
