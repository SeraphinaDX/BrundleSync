//go:build !android

// SPDX-License-Identifier: GPL-3.0-or-later
package main

import "errors"

type platformBackend struct{}

func (platformBackend) Request(string, any, any) error {
	return errors.New("install the Android APK to use the Android bridge")
}
func (platformBackend) Launch(string) error {
	return errors.New("install the Android APK to scan QR codes or choose a camera folder")
}
