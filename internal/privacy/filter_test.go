package privacy

// Coverage contract:
//
// Redact:
// - redacts OpenAI API keys (sk-proj-..., sk-...)
// - redacts Anthropic API keys (sk-ant-...)
// - redacts generic Bearer tokens
// - redacts AWS access keys (AKIA...)
// - redacts GitHub tokens (ghp_..., gho_..., ghs_..., github_pat_...)
// - redacts generic password= assignments
// - redacts private keys (-----BEGIN...PRIVATE KEY-----)
// - preserves non-sensitive content unchanged
// - handles empty input
// - handles content with no matches
//
// DefaultRules:
// - returns non-empty slice of rules
//
// Custom rules:
// - caller can add additional patterns

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilter_RedactsOpenAIKeys(t *testing.T) {
	// Arrange
	content := "export OPENAI_API_KEY=sk-proj-abc123def456ghi789jkl012mno345"
	f := NewFilter(DefaultRules())

	// Act
	redacted := f.Redact(content)

	// Assert
	assert.NotContains(t, redacted, "sk-proj-abc123def456ghi789jkl012mno345")
	assert.Contains(t, redacted, "[REDACTED]")
}

func TestFilter_RedactsAnthropicKeys(t *testing.T) {
	// Arrange
	content := "ANTHROPIC_API_KEY=sk-ant-api03-aBcDeFgHiJkLmNoPqRsTuVwXyZ"
	f := NewFilter(DefaultRules())

	// Act
	redacted := f.Redact(content)

	// Assert
	assert.NotContains(t, redacted, "sk-ant-api03")
	assert.Contains(t, redacted, "[REDACTED]")
}

func TestFilter_RedactsBearerTokens(t *testing.T) {
	// Arrange
	content := `Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1c2VyIn0.signature`
	f := NewFilter(DefaultRules())

	// Act
	redacted := f.Redact(content)

	// Assert
	assert.NotContains(t, redacted, "eyJhbGciOiJIUzI1NiJ9")
	assert.Contains(t, redacted, "[REDACTED]")
}

func TestFilter_RedactsAWSKeys(t *testing.T) {
	// Arrange
	content := "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE"
	f := NewFilter(DefaultRules())

	// Act
	redacted := f.Redact(content)

	// Assert
	assert.NotContains(t, redacted, "AKIAIOSFODNN7EXAMPLE")
	assert.Contains(t, redacted, "[REDACTED]")
}

func TestFilter_RedactsGitHubTokens(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"personal access token", "GITHUB_TOKEN=ghp_ABCdefGHI123456789jklMNOpqrSTUv"},
		{"github_pat", "token=github_pat_11AABBCC22_xyzxyzxyzxyzxyz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := NewFilter(DefaultRules())

			// Act
			redacted := f.Redact(tt.content)

			// Assert
			assert.Contains(t, redacted, "[REDACTED]")
		})
	}
}

func TestFilter_RedactsPasswordAssignments(t *testing.T) {
	// Arrange
	content := `password=mysecretpassword123`
	f := NewFilter(DefaultRules())

	// Act
	redacted := f.Redact(content)

	// Assert
	assert.NotContains(t, redacted, "mysecretpassword123")
	assert.Contains(t, redacted, "[REDACTED]")
}

func TestFilter_RedactsPrivateKeys(t *testing.T) {
	// Arrange
	content := `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA0Z3VS5JJcds3xfn/yB5h
-----END RSA PRIVATE KEY-----`
	f := NewFilter(DefaultRules())

	// Act
	redacted := f.Redact(content)

	// Assert
	assert.NotContains(t, redacted, "MIIEpAIBAAKCAQEA0Z3VS5JJcds3xfn")
	assert.Contains(t, redacted, "[REDACTED]")
}

func TestFilter_PreservesNonSensitiveContent(t *testing.T) {
	// Arrange
	content := "This is a normal commit message about adding a feature."
	f := NewFilter(DefaultRules())

	// Act
	redacted := f.Redact(content)

	// Assert
	assert.Equal(t, content, redacted)
}

func TestFilter_EmptyInput(t *testing.T) {
	// Arrange
	f := NewFilter(DefaultRules())

	// Act
	redacted := f.Redact("")

	// Assert
	assert.Equal(t, "", redacted)
}

func TestDefaultRules_NonEmpty(t *testing.T) {
	// Arrange & Act
	rules := DefaultRules()

	// Assert
	require.NotEmpty(t, rules)
}

func TestFilter_CustomRule(t *testing.T) {
	// Arrange
	rules := append(DefaultRules(), Rule{
		Name:        "internal-url",
		Pattern:     `https://internal\.corp\.example\.com\S*`,
		Replacement: "[INTERNAL_URL]",
	})
	f := NewFilter(rules)

	// Act
	redacted := f.Redact("Visit https://internal.corp.example.com/admin/secrets for details")

	// Assert
	assert.NotContains(t, redacted, "internal.corp.example.com")
	assert.Contains(t, redacted, "[INTERNAL_URL]")
}
