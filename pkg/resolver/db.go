package resolver

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	whereExpired    = "expired_at < ?"
	whereNotExpired = "? <= expired_at"
)

// GitRepo represents a GORM model for git tag data
type GitTag struct {
	gorm.Model
	ID uint

	// RepoID is the GitHub repository ID formatted as "owner/name"
	RepoID string

	// Tag is the git tag name
	Tag string

	// BaseTag is the base tag name
	BaseTag string

	// CommitHash is the git commit hash that the tag points to
	CommitHash string

	// TagHash is the git tag hash
	TagHash string

	// ExpiredAt is the time when the record is expired
	ExpiredAt time.Time
}

func (g GitTag) String() string {
	return fmt.Sprintf("RepoID=%s, Tag=%s, CommitHash=%s, TagHash=%s", g.RepoID, g.Tag, g.CommitHash, g.TagHash)
}

var gormLogger = logger.New(
	log.New(os.Stdout, "\n", log.LstdFlags), // io writer
	logger.Config{
		IgnoreRecordNotFoundError: true, // Ignore ErrRecordNotFound error for logger
	},
)
