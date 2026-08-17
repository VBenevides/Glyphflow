//go:build workerui && !linux

package main

func trayAvailable() bool { return true }
