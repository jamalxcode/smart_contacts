//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// scFitWindow animates the window to a content size of w×h points. The right edge stays where it
// is (the front cover swings open to the left, like a real organiser) and the top edge stays put;
// then the frame is pulled inside the screen's visible area (below the menu bar, above the Dock),
// shrinking it if the screen is too small.
static void scFitWindow(uintptr_t handle, double w, double h, int animate) {
	NSWindow *win = (__bridge NSWindow *)(void *)handle;
	if (win == nil) return;
	NSScreen *scr = win.screen ? win.screen : [NSScreen mainScreen];
	NSRect vis = scr.visibleFrame;
	NSRect cur = win.frame;
	NSRect want = [win frameRectForContentRect:NSMakeRect(0, 0, w, h)];
	double fw = MIN(want.size.width, vis.size.width);
	double fh = MIN(want.size.height, vis.size.height);
	double x = NSMaxX(cur) - fw;
	double y = NSMaxY(cur) - fh;
	x = MAX(vis.origin.x, MIN(x, NSMaxX(vis) - fw));
	y = MAX(vis.origin.y, MIN(y, NSMaxY(vis) - fh));
	NSRect r = NSMakeRect(round(x), round(y), round(fw), round(fh));
	if (animate) {
		[NSAnimationContext runAnimationGroup:^(NSAnimationContext *ctx) {
			ctx.duration = 0.28;
			ctx.timingFunction = [CAMediaTimingFunction functionWithName:kCAMediaTimingFunctionEaseInEaseOut];
			[[win animator] setFrame:r display:YES];
		} completionHandler:nil];
	} else {
		[win setFrame:r display:YES];
	}
}

// scDarkChrome gives the window a dark grey title bar that blends with the dark grey behind the book.
static void scDarkChrome(uintptr_t handle, int dark) {
	NSWindow *win = (__bridge NSWindow *)(void *)handle;
	if (win == nil) return;
	if (dark) {
		win.titlebarAppearsTransparent = YES;
		win.backgroundColor = [NSColor colorWithSRGBRed:0.17 green:0.17 blue:0.18 alpha:1];
		win.appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
	} else {
		win.titlebarAppearsTransparent = NO;
		win.backgroundColor = [NSColor windowBackgroundColor];
		win.appearance = nil;
	}
}
*/
import "C"

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
)

func nsWindow(w fyne.Window) uintptr {
	var h uintptr
	if nw, ok := w.(driver.NativeWindow); ok {
		nw.RunNative(func(ctx any) {
			if m, ok := ctx.(driver.MacWindowContext); ok {
				h = m.NSWindow
			}
		})
	}
	return h
}

// fitWindow resizes the window to sz (animated when asked), keeping it fully on screen.
// It reports false when the native window isn't there yet, so the caller can fall back.
func fitWindow(w fyne.Window, sz fyne.Size, animate bool) bool {
	h := nsWindow(w)
	if h == 0 {
		return false
	}
	a := 0
	if animate {
		a = 1
	}
	C.scFitWindow(C.uintptr_t(h), C.double(sz.Width), C.double(sz.Height), C.int(a))
	return true
}

func darkChrome(w fyne.Window, dark bool) {
	if h := nsWindow(w); h != 0 {
		d := 0
		if dark {
			d = 1
		}
		C.scDarkChrome(C.uintptr_t(h), C.int(d))
	}
}
