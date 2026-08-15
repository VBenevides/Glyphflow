# Glyphflow worker builds

The default `go run ./cmd/worker` entry point is headless and uses SIGINT/SIGTERM for shutdown. It has no graphical-session dependency.

The desktop worker uses the pinned Wails v3 prerelease with the `workerui` build tag. It starts hidden in the system tray; **Open** shows it, window close/minimize hides it, and tray **Exit** performs shutdown. On Linux, if no StatusNotifier tray host is available, it starts as a normal visible window so it can be minimized and restored from the taskbar; closing that fallback window shuts down the worker.

```sh
GOOS=linux GOARCH=amd64 go build -tags workerui -trimpath -ldflags='-s -w' -o bin/glyphflow-worker ./cmd/worker
```

Desktop builds require an active desktop session, GTK/WebKit on Linux, and WebView2 on Windows. Use the `*-headless` artifact for VMs and services.
