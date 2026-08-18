# Glyphflow worker builds

The default `go run ./cmd/worker` entry point is headless and uses SIGINT/SIGTERM for shutdown. It has no graphical-session dependency.

The desktop worker uses Gio with the `workerui` build tag. It starts visible with the same status, log, filter, and tray controls as the former Wails UI. Closing the window exits the worker.

```sh
GOOS=linux GOARCH=amd64 go build -tags workerui -trimpath -ldflags='-s -w' -o bin/glyphflow-worker ./cmd/worker
```

Desktop builds require an active graphical session and GPU-capable desktop libraries. Use the `*-headless` artifact for VMs and services.
