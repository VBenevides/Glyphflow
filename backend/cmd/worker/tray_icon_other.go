//go:build workerui && !windows

package main

var gioTrayIcon = gioWorkerIcon

func handleGioNativeEvent(any) {}
