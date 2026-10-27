//go:build linux

package main

/*
#cgo pkg-config: gtk4
#include <stdlib.h>
#include <gtk/gtk.h>

// Opens uri with the default handler through a GDK launch context, which hands
// the browser an XDG activation token on Wayland (a startup ID on X11), so the
// compositor may bring it to the front. Plain xdg-open passes no token and the
// browser opens a tab in the background. Returns NULL or an error to free.
static char *herakleo_open_uri(const char *uri) {
	GdkDisplay *display = gdk_display_get_default();
	GAppLaunchContext *ctx = display ? G_APP_LAUNCH_CONTEXT(gdk_display_get_app_launch_context(display)) : NULL;
	GError *err = NULL;
	g_app_info_launch_default_for_uri(uri, ctx, &err);
	if (ctx) g_object_unref(ctx);
	if (!err) return NULL;
	char *msg = g_strdup(err->message);
	g_error_free(err);
	return msg;
}
*/
import "C"

import (
	"errors"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// openURL must run on the GTK thread: GDK asks the compositor for the token.
func openURL(u string) error {
	return application.InvokeSyncWithError(func() error {
		cu := C.CString(u)
		defer C.free(unsafe.Pointer(cu))
		if msg := C.herakleo_open_uri(cu); msg != nil {
			defer C.g_free(C.gpointer(unsafe.Pointer(msg)))
			return errors.New(C.GoString(msg))
		}
		return nil
	})
}
