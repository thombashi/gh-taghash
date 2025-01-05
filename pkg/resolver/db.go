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

type GitTag struct {
	gorm.Model
	ID         uint
	RepoID     string
	Tag        string
	BaseTag    string
	CommitHash string
	TagHash    string
	ExpiredAt  time.Time
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
