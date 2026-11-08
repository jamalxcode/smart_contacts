//go:build !darwin

package main

import "fyne.io/fyne/v2"

// On other systems Fyne can't move windows or read the screen size; the caller resizes instead.
func fitWindow(w fyne.Window, sz fyne.Size, animate bool) bool { return false }

func darkChrome(w fyne.Window, dark bool) {}

// setMenuTitle: elsewhere Fyne draws the menu itself, and its titles stay as built.
func setMenuTitle(prefix, title string) {}
