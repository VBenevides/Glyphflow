package store

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type ImportedTaskDefinition struct {
	Name             string
	RunnerID         string
	Schedule         string
	Timezone         string
	Command          []string
	WorkingDirectory string
	Resource         string
}

// ImportSQLiteTaskDefinitions reads both scheduled and manual task tables from
// the local scheduler without modifying the source database.
func ImportSQLiteTaskDefinitions(path string) ([]ImportedTaskDefinition, error) {
	query := `SELECT task_name, runner_identifier, start_time, frequency, cron_expression,
command, script_directory, exclusive_resource FROM task_scheduled
UNION ALL
SELECT task_name, runner_identifier, start_time, '', '', command, script_directory,
exclusive_resource FROM task_spot`
	output, err := exec.Command("sqlite3", "-readonly", "-json", path, query).Output()
	if err != nil {
		return nil, fmt.Errorf("read SQLite task database: %w", err)
	}
	var rows []struct {
		TaskName          string `json:"task_name"`
		RunnerID          string `json:"runner_identifier"`
		StartTime         string `json:"start_time"`
		Frequency         string `json:"frequency"`
		CronExpression    string `json:"cron_expression"`
		Command           string `json:"command"`
		ScriptDirectory   string `json:"script_directory"`
		ExclusiveResource string `json:"exclusive_resource"`
	}
	if err := json.Unmarshal(output, &rows); err != nil {
		return nil, fmt.Errorf("decode SQLite task rows: %w", err)
	}
	tasks := make([]ImportedTaskDefinition, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Command) == "" {
			return nil, fmt.Errorf("SQLite task row has invalid columns or an empty command")
		}
		command, err := parseImportedCommand(row.Command)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, ImportedTaskDefinition{
			Name:             row.TaskName,
			RunnerID:         row.RunnerID,
			Schedule:         strings.TrimSpace(strings.Join([]string{row.Frequency, row.StartTime, row.CronExpression}, " ")),
			Timezone:         "Local",
			Command:          command,
			WorkingDirectory: row.ScriptDirectory,
			Resource:         row.ExclusiveResource,
		})
	}
	return tasks, nil
}

func parseImportedCommand(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("SQLite task command is empty")
	}
	if strings.HasPrefix(value, "[") {
		var command []string
		if err := json.Unmarshal([]byte(value), &command); err != nil || len(command) == 0 {
			return nil, fmt.Errorf("SQLite task command must be a non-empty JSON argument array")
		}
		for _, arg := range command {
			if arg == "" {
				return nil, fmt.Errorf("SQLite task command contains an empty argument")
			}
		}
		return command, nil
	}
	if strings.IndexFunc(value, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' }) >= 0 {
		return nil, fmt.Errorf("SQLite task command with arguments must use a JSON array")
	}
	return []string{value}, nil
}
