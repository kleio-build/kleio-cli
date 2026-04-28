package privacy

import "regexp"

type Rule struct {
	Name        string
	Pattern     string
	Replacement string
	compiled    *regexp.Regexp
}

type Filter struct {
	rules []compiledRule
}

type compiledRule struct {
	name        string
	re          *regexp.Regexp
	replacement string
}

func NewFilter(rules []Rule) *Filter {
	f := &Filter{}
	for _, r := range rules {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			continue
		}
		replacement := r.Replacement
		if replacement == "" {
			replacement = "[REDACTED]"
		}
		f.rules = append(f.rules, compiledRule{
			name:        r.Name,
			re:          re,
			replacement: replacement,
		})
	}
	return f
}

func (f *Filter) Redact(content string) string {
	result := content
	for _, r := range f.rules {
		result = r.re.ReplaceAllString(result, r.replacement)
	}
	return result
}

func DefaultRules() []Rule {
	return []Rule{
		{
			Name:    "openai-key",
			Pattern: `sk-(?:proj-)?[A-Za-z0-9_-]{20,}`,
		},
		{
			Name:    "anthropic-key",
			Pattern: `sk-ant-[A-Za-z0-9_-]{20,}`,
		},
		{
			Name:    "bearer-token",
			Pattern: `Bearer\s+[A-Za-z0-9_.\-]+`,
		},
		{
			Name:    "aws-access-key",
			Pattern: `AKIA[0-9A-Z]{16}`,
		},
		{
			Name:    "github-token",
			Pattern: `(?:ghp|gho|ghs|ghr)_[A-Za-z0-9_]{30,}`,
		},
		{
			Name:    "github-pat",
			Pattern: `github_pat_[A-Za-z0-9_]{20,}`,
		},
		{
			Name:    "password-assignment",
			Pattern: `(?i)password\s*=\s*\S+`,
		},
		{
			Name:    "private-key-block",
			Pattern: `(?s)-----BEGIN[A-Z ]*PRIVATE KEY-----.*?-----END[A-Z ]*PRIVATE KEY-----`,
		},
	}
}
