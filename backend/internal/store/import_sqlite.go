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
		tasks = append(tasks, ImportedTaskDefinition{
			Name:             row.TaskName,
			RunnerID:         row.RunnerID,
			Schedule:         strings.TrimSpace(strings.Join([]string{row.Frequency, row.StartTime, row.CronExpression}, " ")),
			Timezone:         "Local",
			Command:          []string{row.Command},
			WorkingDirectory: row.ScriptDirectory,
			Resource:         row.ExclusiveResource,
		})
	}
	return tasks, nil
}
