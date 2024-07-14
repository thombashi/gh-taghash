package resolver

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCacheTTL(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	ttl := "1h"

	got, err := ParseCacheTTL(ttl)
	r.NoError(err)

	a.Equal(7*time.Minute+30*time.Second, got.GitAliasTagTTL)
	a.Equal(30*time.Hour, got.GitFileTTL)
	a.Equal(time.Hour, got.GitTagTTL)
	a.Equal(30*time.Minute, got.QueryTTL)
}
