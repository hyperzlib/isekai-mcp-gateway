package wiki

import "time"

type Object = map[string]any
type PageRef struct {
	PageID int64  `json:"page_id,omitempty"`
	Title  string `json:"title,omitempty"`
}
type Operation struct {
	Type      string `json:"type"`
	Content   string `json:"content,omitempty"`
	OldText   string `json:"old_text,omitempty"`
	NewText   string `json:"new_text,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	Line      int    `json:"line,omitempty"`
	Position  string `json:"position,omitempty"`
	Patch     string `json:"patch,omitempty"`
}
type Response struct {
	ChallengeID string `json:"challenge_id"`
	Confirmed   bool   `json:"confirmed"`
}
type Input struct {
	Titles                  []string    `json:"titles,omitempty"`
	Page                    PageRef     `json:"page,omitempty"`
	Title                   string      `json:"title,omitempty"`
	PageID                  int64       `json:"page_id,omitempty"`
	Format                  string      `json:"format,omitempty"`
	Offset                  int         `json:"offset,omitempty"`
	Limit                   int         `json:"limit,omitempty"`
	Cursor                  string      `json:"cursor,omitempty"`
	Query                   string      `json:"query,omitempty"`
	User                    string      `json:"user,omitempty"`
	Start                   string      `json:"start,omitempty"`
	End                     string      `json:"end,omitempty"`
	Category                string      `json:"category,omitempty"`
	UserID                  int64       `json:"user_id,omitempty"`
	Username                string      `json:"username,omitempty"`
	Direction               string      `json:"direction,omitempty"`
	Namespaces              []int       `json:"namespaces,omitempty"`
	Show                    []string    `json:"show,omitempty"`
	Tag                     string      `json:"tag,omitempty"`
	Types                   []string    `json:"types,omitempty"`
	DependencyTypes         []string    `json:"dependency_types,omitempty"`
	Language                string      `json:"language,omitempty"`
	RevisionID              int64       `json:"revision_id,omitempty"`
	FromRevision            int64       `json:"from_revision,omitempty"`
	ToRevision              int64       `json:"to_revision,omitempty"`
	EditID                  string      `json:"edit_id,omitempty"`
	BaseVersion             int64       `json:"base_version,omitempty"`
	ExpectedVersion         int64       `json:"expected_version,omitempty"`
	FollowRedirect          bool        `json:"follow_redirect,omitempty"`
	FollowRedirects         bool        `json:"follow_redirects,omitempty"`
	StartLine               int         `json:"start_line,omitempty"`
	LineCount               int         `json:"line_count,omitempty"`
	IncludeLineNumbers      *bool       `json:"include_line_numbers,omitempty"`
	Pattern                 string      `json:"pattern,omitempty"`
	Regex                   bool        `json:"regex,omitempty"`
	CaseSensitive           *bool       `json:"case_sensitive,omitempty"`
	ContextLines            *int        `json:"context_lines,omitempty"`
	MaxMatches              int         `json:"max_matches,omitempty"`
	Operations              []Operation `json:"operations,omitempty"`
	Comparison              string      `json:"comparison,omitempty"`
	ExpectedCurrentRevision int64       `json:"expected_current_revision,omitempty"`
	Summary                 string      `json:"summary,omitempty"`
	AIInvolvement           string      `json:"ai_involvement,omitempty"`
	ChallengeResponses      []Response  `json:"challenge_responses,omitempty"`
	ToTitle                 string      `json:"to_title,omitempty"`
	Reason                  string      `json:"reason,omitempty"`
	Filename                string      `json:"filename,omitempty"`
	MIMEType                string      `json:"mime_type,omitempty"`
	SizeBytes               int64       `json:"size_bytes,omitempty"`
	SHA256                  string      `json:"sha256,omitempty"`
	Comment                 string      `json:"comment,omitempty"`
	Overwrite               bool        `json:"overwrite,omitempty"`
	RetryUploadID           string      `json:"retry_upload_id,omitempty"`
	UploadID                string      `json:"upload_id,omitempty"`
	ConfirmedWarnings       []string    `json:"confirmed_warnings,omitempty"`
}
type Edit struct {
	ID                       string     `json:"edit_id"`
	Owner                    string     `json:"-"`
	SiteID                   string     `json:"-"`
	Kind                     string     `json:"kind"`
	PageID                   int64      `json:"page_id"`
	Title                    string     `json:"title"`
	URL                      string     `json:"url"`
	BaseRevisionID           int64      `json:"base_revision_id"`
	ModerationBaseRevisionID *int64     `json:"moderation_base_revision_id,omitempty"`
	StartTimestamp           string     `json:"-"`
	Origin                   string     `json:"-"`
	Content                  string     `json:"-"`
	Source                   string     `json:"content_source"`
	Version                  int64      `json:"version"`
	State                    string     `json:"state"`
	PreviousState            string     `json:"-"`
	Operation                string     `json:"-"`
	CloseRequested           bool       `json:"close_requested"`
	EndedAt                  *time.Time `json:"ended_at"`
	CreatedAt                time.Time  `json:"created_at"`
	ExpiresAt                time.Time  `json:"expires_at"`
	Marker                   string     `json:"-"`
	Error                    Object     `json:"error,omitempty"`
	HasChanges               bool       `json:"has_changes"`
	EndReason                string     `json:"end_reason,omitempty"`
}

// Payload uses internal field names so private draft fields survive persistence.
type editPayload struct {
	Edit                                                                             Edit
	Origin, Content, Owner, SiteID, StartTimestamp, PreviousState, Operation, Marker string
}
type Attempt struct {
	EditID      string
	Version     int64
	Sent        bool
	Result      Object
	Reason      string
	ChallengeID string
}
type Challenge struct {
	ID, Owner, Tool, Fingerprint, State string
	Params                              Object
	ExpiresAt                           time.Time
}
type Ticket struct {
	ID, Owner, GrantID, SiteID, Filename, MIMEType, SHA256, Comment, State, Path, Fingerprint string
	SizeBytes                                                                                 int64
	Overwrite                                                                                 bool
	ExpiresAt, StagedExpiresAt, ResultExpiresAt                                               time.Time
	Warnings                                                                                  Object
	Result                                                                                    Object
	Sent                                                                                      bool
	ReceivingPath                                                                             string
	PreviousState                                                                             string
	ChallengeID                                                                               string
	StagedTTL                                                                                 int
	RetentionTTL                                                                              int
}
type LineRange struct {
	Start int `json:"start_line"`
	End   int `json:"end_line"`
}
