//go:build !linux

package main

import "github.com/wailsapp/wails/v3/pkg/application"

func openURL(u string) error { return application.Get().Browser.OpenURL(u) }
