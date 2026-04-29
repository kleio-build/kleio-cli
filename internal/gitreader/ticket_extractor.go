package gitreader

import (
	"regexp"
)

var (
	jiraPattern   = regexp.MustCompile(`\b([A-Z][A-Z0-9]+-\d+)\b`)
	githubPattern = regexp.MustCompile(`(?:^|[\s(])(#\d+)\b`)
	klPattern     = regexp.MustCompile(`\b(KL-\d+)\b`)
)

type TicketExtractor struct{}

func NewTicketExtractor() *TicketExtractor {
	return &TicketExtractor{}
}

func (e *TicketExtractor) Extract(branch, message string) []string {
	seen := make(map[string]bool)
	var tickets []string

	add := func(t string) {
		if !seen[t] {
			seen[t] = true
			tickets = append(tickets, t)
		}
	}

	for _, m := range jiraPattern.FindAllString(branch, -1) {
		add(m)
	}
	for _, m := range klPattern.FindAllString(branch, -1) {
		add(m)
	}

	for _, m := range jiraPattern.FindAllString(message, -1) {
		add(m)
	}
	for _, m := range klPattern.FindAllString(message, -1) {
		add(m)
	}
	for _, matches := range githubPattern.FindAllStringSubmatch(message, -1) {
		if len(matches) > 1 {
			add(matches[1])
		}
	}

	return tickets
}
