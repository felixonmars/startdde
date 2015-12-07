package main

// #cgo pkg-config: gtk+-3.0
// #include <gtk/gtk.h>
// void gtkInit() {
//    gtk_init(NULL, NULL);
// }
import "C"
import (
	"flag"
	"pkg.deepin.io/dde/api/soundutils"
	"pkg.deepin.io/lib/log"
	"pkg.deepin.io/lib/proxy"
)

var logger = log.NewLogger("com.deepin.SessionManager")

var debug = flag.Bool("d", false, "debug")
var windowManagerBin = flag.String("wm", "/usr/bin/deepin-wm", "the window manager used by dde")

func main() {
	C.gtkInit()
	flag.Parse()

	soundutils.PlaySystemSound(soundutils.EventLogin, "", false)

	proxy.SetupProxy()

	startXSettings()

	startDisplay()

	startSession()

	C.gtk_main()
}
