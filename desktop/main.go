// Package desktop is the DAWGit desktop app: a tray app that runs the agent
// for each project and a window to commit versions, get updates and manage
// branches. cmd/dawgit-desktop runs it, and so can a build with extensions
// (see dawgit/ext).
package desktop

import (
	"embed"
	"log"
	"os"
	"slices"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/windows/icon.ico
var trayIcon []byte

// Run starts the app and returns when it quits (extensions register what
// they add first).
func Run() {
	svc := NewApp()
	ns := notifications.New()

	var window *application.WebviewWindow
	showWindow := func() {
		if window != nil {
			window.Show()
			window.Restore()
			window.Focus()
		}
	}

	app := application.New(application.Options{
		Name:        "DAWGit",
		Description: "Version control and collaboration for Ableton Live projects",
		Services: []application.Service{
			application.NewService(svc),
			application.NewService(ns),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: svc.fileServer, // audio previews of project files
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.dawgit.desktop",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				showWindow()
			},
		},
		Windows: application.WindowsOptions{},
	})

	svc.emit = func(name string, data any) { app.Event.Emit(name, data) }
	svc.notify = func(title, body string) {
		if err := ns.SendNotification(notifications.NotificationOptions{ID: title, Title: title, Body: body}); err != nil {
			log.Printf("notification: %v", err)
		}
	}
	svc.openURL = func(url string) error { return app.Browser.OpenURL(url) }
	svc.pickDir = func(title string) (string, error) {
		// Browser (server-mode) testing has no native dialogs.
		if dir := os.Getenv("DAWGIT_DEV_PICK_DIR"); dir != "" {
			return dir, nil
		}
		return app.Dialog.OpenFile().
			CanChooseDirectories(true).
			CanChooseFiles(false).
			CanCreateDirectories(true).
			SetTitle(title).
			PromptForSingleSelection()
	}

	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "DAWGit",
		Width:            1180,
		Height:           760,
		MinWidth:         900,
		MinHeight:        560,
		BackgroundColour: application.NewRGB(24, 25, 29),
		URL:              "/",
		// Started by Windows at sign-in: stay in the tray.
		Hidden: slices.Contains(os.Args[1:], backgroundFlag),
	})
	// Closing the window keeps DAWGit running in the tray (the agents keep
	// watching); Quit is in the tray menu.
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})

	// From the tray (or minimised) the agents look for new versions less often.
	svc.setHidden(slices.Contains(os.Args[1:], backgroundFlag))
	for ev, hidden := range map[events.WindowEventType]bool{
		events.Common.WindowHide: true, events.Common.WindowMinimise: true,
		events.Common.WindowShow: false, events.Common.WindowRestore: false,
	} {
		window.OnWindowEvent(ev, func(*application.WindowEvent) { svc.setHidden(hidden) })
	}

	menu := app.NewMenu()
	menu.Add("Open DAWGit").OnClick(func(*application.Context) { showWindow() })
	menu.AddSeparator()
	menu.Add("Quit").OnClick(func(*application.Context) { app.Quit() })

	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetTooltip("DAWGit")
	tray.SetMenu(menu)
	tray.OnClick(showWindow)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
