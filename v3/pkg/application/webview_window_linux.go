//go:build linux

package application

import (
	"fmt"
	"time"

	"github.com/bep/debounce"
	"github.com/wailsapp/wails/v3/internal/assetserver"
	"github.com/wailsapp/wails/v3/internal/capabilities"
	"github.com/wailsapp/wails/v3/internal/runtime"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	windowDidMoveDebounceMS = 200
)

type dragInfo struct {
	XRoot       int
	YRoot       int
	DragTime    uint32
	MouseButton uint
}

type linuxWebviewWindow struct {
	id            uint
	application   pointer
	window        pointer
	webview       pointer
	parent        *WebviewWindow
	menubar       pointer
	vbox          pointer
	menu          *Menu
	accels        pointer
	lastWidth     int
	lastHeight    int
	drag          dragInfo
	lastX, lastY  int
	gtkmenu       pointer
	ctxMenuOpened bool

	moveDebouncer     func(func())
	resizeDebouncer   func(func())
	ignoreMouseEvents bool
}

var (
	registered bool = false // avoid 'already registered message' about 'wails://'
)

func (w *linuxWebviewWindow) endDrag(button uint, x, y int) {
	w.drag.XRoot = 0.0
	w.drag.YRoot = 0.0
	w.drag.DragTime = 0
}

func (w *linuxWebviewWindow) connectSignals() {
	cb := func(e events.WindowEventType) {
		w.parent.emit(e)
	}
	w.setupSignalHandlers(cb)
}

func (w *linuxWebviewWindow) openContextMenu(menu *Menu, data *ContextMenuData) {
	// Create the menu manually because we don't want a gtk_menu_bar
	// as the top-level item
	ctxMenu := &linuxMenu{
		menu: menu,
	}
	if menu.impl == nil {
		ctxMenu.update()

		native := ctxMenu.menu.impl.(*linuxMenu).native
		w.contextMenuSignals(native)
	}

	native := ctxMenu.menu.impl.(*linuxMenu).native
	w.contextMenuShow(native, data)
}

func (w *linuxWebviewWindow) focus() {
	w.present()
}

func (w *linuxWebviewWindow) isNormal() bool {
	return !w.isMinimised() && !w.isMaximised() && !w.isFullscreen()
}

func (w *linuxWebviewWindow) setCloseButtonEnabled(enabled bool) {
	//	C.enableCloseButton(w.nsWindow, C.bool(enabled))
}

func (w *linuxWebviewWindow) setFullscreenButtonEnabled(enabled bool) {
	// Not implemented
}

func (w *linuxWebviewWindow) setMinimiseButtonEnabled(enabled bool) {
	//C.enableMinimiseButton(w.nsWindow, C.bool(enabled))
}

func (w *linuxWebviewWindow) setMaximiseButtonEnabled(enabled bool) {
	//C.enableMaximiseButton(w.nsWindow, C.bool(enabled))
}

func (w *linuxWebviewWindow) disableSizeConstraints() {
	x, y, width, height, scale := w.getCurrentMonitorGeometry()
	w.setMinMaxSize(x, y, width*scale, height*scale)
}

func (w *linuxWebviewWindow) unminimise() {
	w.present()
}

func (w *linuxWebviewWindow) on(eventID uint) {
	// TODO: Test register/unregister listener for linux events
	//C.registerListener(C.uint(eventID))
}

func (w *linuxWebviewWindow) zoom() {
	w.zoomIn()
}

func (w *linuxWebviewWindow) windowZoom() {
	w.zoom() // FIXME> This should be removed
}

func (w *linuxWebviewWindow) forceReload() {
	w.reload()
}

func (w *linuxWebviewWindow) center() {
	x, y, width, height, _ := w.getCurrentMonitorGeometry()
	if x == -1 && y == -1 && width == -1 && height == -1 {
		return
	}
	windowWidth, windowHeight := w.size()

	newX := ((width - windowWidth) / 2) + x
	newY := ((height - windowHeight) / 2) + y

	// Place the window at the center of the monitor
	w.move(newX, newY)
}

func (w *linuxWebviewWindow) restore() {
	// restore window to normal size
	// FIXME: never called!  - remove from webviewImpl interface
}

func newWindowImpl(parent *WebviewWindow) *linuxWebviewWindow {
	//	(*C.struct__GtkWidget)(m.native)
	//var menubar *C.struct__GtkWidget
	result := &linuxWebviewWindow{
		application: getNativeApplication().application,
		parent:      parent,
		//		menubar:     menubar,
	}
	return result
}

func (w *linuxWebviewWindow) setMinMaxSize(minWidth, minHeight, maxWidth, maxHeight int) {
	// Get current screen for window
	_, _, monitorwidth, monitorheight, _ := w.getCurrentMonitorGeometry()
	if maxWidth == 0 {
		maxWidth = monitorwidth
	}
	if maxHeight == 0 {
		maxHeight = monitorheight
	}
	windowSetGeometryHints(w.window, minWidth, minHeight, maxWidth, maxHeight)
}

func (w *linuxWebviewWindow) setMinSize(width, height int) {
	w.setMinMaxSize(width, height, w.parent.options.MaxWidth, w.parent.options.MaxHeight)
}

func (w *linuxWebviewWindow) getBorderSizes() *LRTB {
	return &LRTB{}
}

func (w *linuxWebviewWindow) setMaxSize(width, height int) {
	w.setMinMaxSize(w.parent.options.MinWidth, w.parent.options.MinHeight, width, height)
}

func (w *linuxWebviewWindow) setRelativePosition(x, y int) {
	mx, my, _, _, _ := w.getCurrentMonitorGeometry()
	w.move(x+mx, y+my)
}

func (w *linuxWebviewWindow) width() int {
	width, _ := w.size()
	return width
}

func (w *linuxWebviewWindow) height() int {
	_, height := w.size()
	return height
}

func (w *linuxWebviewWindow) setPosition(x int, y int) {
	// Set the window's absolute position
	w.move(x, y)
}

func (w *linuxWebviewWindow) run() {
	for eventId := range w.parent.eventListeners {
		w.on(eventId)
	}

	if w.moveDebouncer == nil {
		w.moveDebouncer = debounce.New(time.Duration(windowDidMoveDebounceMS) * time.Millisecond)
	}
	if w.resizeDebouncer == nil {
		w.resizeDebouncer = debounce.New(time.Duration(windowDidMoveDebounceMS) * time.Millisecond)
	}

	// Register the capabilities
	globalApplication.capabilities = capabilities.NewCapabilities()

	app := getNativeApplication()

	var menu = w.menu
	if menu == nil && globalApplication.ApplicationMenu != nil {
		menu = globalApplication.ApplicationMenu.Clone()
	}
	if menu != nil {
		InvokeSync(func() {
			menu.Update()
		})
		w.menu = menu
		w.gtkmenu = (menu.impl).(*linuxMenu).native
	}

	windowOptions := w.parent.options
	w.window, w.webview, w.vbox = windowNew(app.application, w.gtkmenu, w.parent.id, windowOptions.Linux.WebviewGpuPolicy)
	app.registerWindow(w.window, w.parent.id) // record our mapping
	w.connectSignals()
	if windowOptions.EnableDragAndDrop {
		w.enableDND()
	}
	w.setTitle(windowOptions.Title)
	w.setIcon(app.icon)
	w.setAlwaysOnTop(windowOptions.AlwaysOnTop)
	w.setResizable(!windowOptions.DisableResize)
	// only set min/max size if actually set
	if windowOptions.MinWidth != 0 &&
		windowOptions.MinHeight != 0 &&
		windowOptions.MaxWidth != 0 &&
		windowOptions.MaxHeight != 0 {
		w.setMinMaxSize(
			windowOptions.MinWidth,
			windowOptions.MinHeight,
			windowOptions.MaxWidth,
			windowOptions.MaxHeight,
		)
	}
	w.setDefaultSize(windowOptions.Width, windowOptions.Height)
	w.setSize(windowOptions.Width, windowOptions.Height)
	w.setZoom(windowOptions.Zoom)
	if windowOptions.BackgroundType != BackgroundTypeSolid {
		w.setTransparent()
		w.setBackgroundColour(windowOptions.BackgroundColour)
	}

	w.setFrameless(windowOptions.Frameless)

	if windowOptions.X != 0 || windowOptions.Y != 0 {
		w.setRelativePosition(windowOptions.X, windowOptions.Y)
	} else {
		w.center()
	}
	switch windowOptions.StartState {
	case WindowStateMaximised:
		w.maximise()
	case WindowStateMinimised:
		w.minimise()
	case WindowStateFullscreen:
		w.fullscreen()
	case WindowStateNormal:
	}

	// Ignore mouse events if requested
	w.setIgnoreMouseEvents(windowOptions.IgnoreMouseEvents)

	startURL, err := assetserver.GetStartURL(windowOptions.URL)
	if err != nil {
		globalApplication.fatal(err.Error())
	}

	w.setURL(startURL)
	w.parent.OnWindowEvent(events.Linux.WindowLoadChanged, func(_ *WindowEvent) {
		if windowOptions.JS != "" {
			w.execJS(windowOptions.JS)
		}
		if windowOptions.CSS != "" {
			js := fmt.Sprintf("(function() { var style = document.createElement('style'); style.appendChild(document.createTextNode('%s')); document.head.appendChild(style); })();", windowOptions.CSS)
			w.execJS(js)
		}
	})
	w.parent.OnWindowEvent(events.Linux.WindowFocusIn, func(e *WindowEvent) {
		w.parent.emit(events.Common.WindowFocus)
	})
	w.parent.OnWindowEvent(events.Linux.WindowFocusOut, func(e *WindowEvent) {
		w.parent.emit(events.Common.WindowLostFocus)
	})
	w.parent.OnWindowEvent(events.Linux.WindowDeleteEvent, func(e *WindowEvent) {
		w.parent.emit(events.Common.WindowClosing)
	})
	w.parent.OnWindowEvent(events.Linux.WindowDidMove, func(e *WindowEvent) {
		w.parent.emit(events.Common.WindowDidMove)
	})
	w.parent.OnWindowEvent(events.Linux.WindowDidResize, func(e *WindowEvent) {
		w.parent.emit(events.Common.WindowDidResize)
	})

	w.parent.RegisterHook(events.Linux.WindowLoadChanged, func(e *WindowEvent) {
		w.execJS(runtime.Core())
	})
	if windowOptions.HTML != "" {
		w.setHTML(windowOptions.HTML)
	}
	if !windowOptions.Hidden {
		w.show()
		if windowOptions.X != 0 || windowOptions.Y != 0 {
			w.setRelativePosition(windowOptions.X, windowOptions.Y)
		} else {
			w.center() // needs to be queued until after GTK starts up!
		}
	}
	if windowOptions.DevToolsEnabled || globalApplication.isDebugMode {
		w.enableDevTools()
		if windowOptions.OpenInspectorOnStartup {
			w.openDevTools()
		}
	}
}

func (w *linuxWebviewWindow) startResize(border string) error {
	// FIXME: what do we need to do here?
	return nil
}

func (w *linuxWebviewWindow) nativeWindowHandle() uintptr {
	return uintptr(w.window)
}

func (w *linuxWebviewWindow) print() error {
	w.execJS("window.print();")
	return nil
}

func (w *linuxWebviewWindow) handleKeyEvent(acceleratorString string) {
	// Parse acceleratorString
	// accelerator, err := parseAccelerator(acceleratorString)
	// if err != nil {
	// 	globalApplication.error("unable to parse accelerator: %s", err.Error())
	// 	return
	// }
	w.parent.processKeyBinding(acceleratorString)
}

// SetMinimiseButtonState is unsupported on Linux
func (w *linuxWebviewWindow) setMinimiseButtonState(state ButtonState) {}

// SetMaximiseButtonState is unsupported on Linux
func (w *linuxWebviewWindow) setMaximiseButtonState(state ButtonState) {}

// SetCloseButtonState is unsupported on Linux
func (w *linuxWebviewWindow) setCloseButtonState(state ButtonState) {}

func (w *linuxWebviewWindow) isIgnoreMouseEvents() bool {
	return w.ignoreMouseEvents
}

func (w *linuxWebviewWindow) setIgnoreMouseEvents(ignore bool) {
	w.ignoreMouse(w.ignoreMouseEvents)
}
