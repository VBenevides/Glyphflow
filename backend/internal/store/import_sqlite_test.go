package store

import (
	"os/exec"
	"testing"
)

func TestImportSQLiteTaskDefinitions(t *testing.T) {
	path := t.TempDir() + "/tasks.sqlite"
	if err := exec.Command("sqlite3", path, `CREATE TABLE task_scheduled (
		task_name TEXT, runner_identifier TEXT, start_time TEXT, frequency TEXT,
		cron_expression TEXT, command TEXT, script_directory TEXT, exclusive_resource TEXT
	); CREATE TABLE task_spot (
		task_name TEXT, runner_identifier TEXT, start_time TEXT, command TEXT, script_directory TEXT, exclusive_resource TEXT
	); INSERT INTO task_scheduled VALUES ('backup', 'worker-1', '02:00', 'Daily', '', 'backup.exe', '/srv', 'db');`).Run(); err != nil {
		t.Fatal(err)
	}
	tasks, err := ImportSQLiteTaskDefinitions(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Name != "backup" || tasks[0].RunnerID != "worker-1" || tasks[0].Command[0] != "backup.exe" {
		t.Fatalf("unexpected imported tasks: %#v", tasks)
	}
}
