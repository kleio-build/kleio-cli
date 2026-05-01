package cursorimport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
)

// workspaceFileSchema is the minimal subset of a .code-workspace file we
// care about: the folders[].path entries.
type workspaceFileSchema struct {
	Folders []struct {
		Path string `json:"path"`
		Name string `json:"name,omitempty"`
	} `json:"folders"`
}

// ParseWorkspaceFolders is the exported alias for parseWorkspaceFolders
// so external packages (e.g. internal/ingest/discovery) can reuse the
// JSON-with-comments parser.
func ParseWorkspaceFolders(data []byte) ([]string, error) { return parseWorkspaceFolders(data) }

// parseWorkspaceFolders accepts the raw bytes of a .code-workspace file
// (JSON, but tolerating // line comments which VS Code permits in practice)
// and returns each folder's path entry in declaration order.
func parseWorkspaceFolders(data []byte) ([]string, error) {
	stripped := stripJSONLineComments(data)
	var ws workspaceFileSchema
	if err := json.Unmarshal(stripped, &ws); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ws.Folders))
	for _, f := range ws.Folders {
		if f.Path != "" {
			out = append(out, f.Path)
		}
	}
	return out, nil
}

// stripJSONLineComments removes // line comments outside of string literals.
// VS Code's settings/workspace files allow these even though strict JSON
// does not. Block comments and trailing commas are left intact: callers
// that hit those will get a json.Unmarshal error and can choose to harden
// later.
func stripJSONLineComments(data []byte) []byte {
	var out bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		out.WriteString(stripLineComment(line))
		out.WriteByte('\n')
	}
	return out.Bytes()
}

func stripLineComment(line string) string {
	inStr := false
	escape := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if escape {
			escape = false
			continue
		}
		if c == '\\' && inStr {
			escape = true
			continue
		}
		if c == '"' {
			inStr = !inStr
			continue
		}
		if !inStr && c == '/' && i+1 < len(line) && line[i+1] == '/' {
			return strings.TrimRight(line[:i], " \t")
		}
	}
	return line
}
