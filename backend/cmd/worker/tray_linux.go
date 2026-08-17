//go:build workerui && linux

package main

import "github.com/godbus/dbus/v5"

func trayAvailable() bool {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false
	}
	defer conn.Close()

	var available bool
	err = conn.Object("org.freedesktop.DBus", "/org/freedesktop/DBus").
		Call("org.freedesktop.DBus.NameHasOwner", 0, "org.kde.StatusNotifierWatcher").
		Store(&available)
	return err == nil && available
}
