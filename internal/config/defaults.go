package config

func Defaults() *Config {
	c := &Config{}
	c.App.Site.Title = DefaultSiteTitle
	c.Static.Version, c.App.Version = 1, 1
	c.Static.Server = Server{Host: "127.0.0.1", Port: 3000, RequestBodyMaxBytes: 2097152}
	g := &c.Static.Auth.Gateway
	g.AccessTokenFormat = "opaque"
	g.CodeTTL, g.AccessTTL, g.InteractionTTL = 60, 300, 600
	g.RefreshTTL, g.GrantTTL = 604800, 2592000
	g.Registration.CIMD, g.Registration.Dynamic = true, true
	g.Registration.MaxClients, g.Registration.IdleTTL = 10000, 2592000
	g.Fetch.TimeoutMS, g.Fetch.MaxBytes, g.Fetch.MaxRedirects = 5000, 65536, 2
	c.Static.Storage = Storage{Driver: "mysql", Host: "127.0.0.1", Port: 3306, PoolSize: 10}
	c.App.Auth.OidcClient.Scopes = []string{"openid", "profile", "email"}
	c.App.Auth.Consent.DefaultSelectedScopes = []string{"mcp:access"}
	c.App.MediaWiki.TimeoutMS = 15000
	c.App.MediaWiki.Maxlag = 5
	c.App.MediaWiki.PageCacheTTL = 600
	c.App.Editing.TTL = 3600
	c.App.Editing.MaxActive = 1000
	c.App.Editing.MaxContent = 16777216
	c.App.Editing.MaxOperations = 64
	c.App.Uploads.TaskTTL = 600
	c.App.Uploads.StagedTTL = 600
	c.App.Uploads.Retention = 3600
	c.App.Uploads.MaxBytes = 67108864
	c.App.Uploads.MaxActive = 10
	c.App.Uploads.ReservedPrincipal = 268435456
	c.App.Uploads.ReservedGlobal = 1073741824
	c.App.Uploads.TempPath = "./data/uploads"
	c.App.Uploads.Mimes = []string{"image/png", "image/jpeg", "image/webp", "image/gif", "image/svg+xml", "application/pdf"}
	c.App.Results.TextMax = 65536
	c.App.Results.PreviewMax = 4194304
	c.App.Additions.Foreign.Exemption = "has_edited"
	c.App.Additions.Foreign.Operations = []string{"commit", "move", "delete", "uploadOverwrite"}
	c.App.Additions.Foreign.TTL = 600
	c.App.Additions.Foreign.Version = 1
	c.App.Additions.Tags.Mapping = map[string]string{"ai_organized": "ai-organized", "ai_assisted": "ai-assisted", "ai_generated": "ai-generated"}
	c.App.Logging.Level = "info"
	c.App.Logging.ConsoleFormat = "text"
	c.App.Logging.File.Path = "./logs/log.jsonl"
	c.App.Logging.File.ErrorPath = "./logs/error.jsonl"
	c.App.Logging.File.Split = true
	return c
}
