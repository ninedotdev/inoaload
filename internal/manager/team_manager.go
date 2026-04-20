//go:build darwin

package manager

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Team is a minimal projection of plumesign's team list output.
type Team struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	FreeTier bool   `json:"free_tier"`
}

// ListAppleIDTeams runs `plumesign account teams -u <email>` and parses the
// result. Plumesign prints teams in a prettified format; we also accept JSON
// if the binary is given the right env flag (future-proof).
func ListAppleIDTeams(ctx context.Context, email string) ([]Team, error) {
	plumesignPath, err := ResolvePlumesign()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, plumesignPath, "account", "teams", "-u", email)
	cmd.Env = augmentedPath(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	return parseTeamsOutput(string(out)), nil
}

// parseTeamsOutput understands both JSON output and plumesign's default text
// rendering: "Name (TEAMID) — Type [free]".
func parseTeamsOutput(out string) []Team {
	trimmed := strings.TrimSpace(out)
	// Try JSON first.
	if strings.HasPrefix(trimmed, "[") {
		var ts []Team
		if err := json.Unmarshal([]byte(trimmed), &ts); err == nil {
			return ts
		}
	}
	// Plumesign log format: " - `<name>`, with ID `<id>`."
	re := regexp.MustCompile("`([^`]+)`,\\s+with ID\\s+`([A-Z0-9]+)`")
	matches := re.FindAllStringSubmatch(trimmed, -1)
	teams := make([]Team, 0, len(matches))
	for _, m := range matches {
		name := strings.TrimSpace(m[1])
		id := m[2]
		teams = append(teams, Team{ID: id, Name: name})
	}
	return teams
}
