package config

import (
	"testing"

	assert "github.com/stretchr/testify/require"
)

func TestCompareVersions(t *testing.T) {
	assert.Negative(t, compareVersions("9.0", "10.0"))
	assert.Negative(t, compareVersions("10.0", "10.1"))
	assert.Positive(t, compareVersions("128.0.6613", "128.0"))
	assert.Zero(t, compareVersions("1.2", "1.2"))
	assert.Positive(t, compareVersions("beta", "alpha"))
}
