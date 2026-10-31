//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

// Windows: the per-user Run key starts the agent at login (no admin rights needed).
const runKeyName = ProductName

// The login item's name before the rename; removed when the new one is written.
const oldRunKeyName = "USCF Print Agent"

func installAutostart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	_ = k.DeleteValue(oldRunKeyName)
	return k.SetStringValue(runKeyName, `"`+exe+`"`)
}

func removeAutostart() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(runKeyName); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}
