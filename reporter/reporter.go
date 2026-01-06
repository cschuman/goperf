package reporter

import "github.com/cschuman/goperf/rules"

// Reporter formats and outputs issues
type Reporter interface {
	Report(issues []rules.Issue) string
}
