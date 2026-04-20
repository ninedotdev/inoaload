package main

import (
	"embed"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/bitxeno/atvloadly/internal/app"
	"github.com/bitxeno/atvloadly/internal/log"
	"github.com/bitxeno/atvloadly/internal/manager"
	"github.com/bitxeno/atvloadly/internal/task"
	"github.com/bitxeno/atvloadly/web"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var reactAssets embed.FS

const fiberPort = 6066

func bootstrapServer() error {
	// Extract the embedded plumesign and prepend its dir to PATH so every
	// `exec.Command("plumesign", ...)` call site resolves to our bundled copy.
	if p, err := manager.ResolvePlumesign(); err == nil && p != "" {
		dir := filepath.Dir(p)
		_ = os.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	}

	conf, err := app.InitConfig("", false)
	if err != nil {
		return err
	}
	if _, err := app.InitSettings(conf, false); err != nil {
		return err
	}
	if err := app.InitLogger(conf); err != nil {
		return err
	}
	if err := app.InitDb(conf); err != nil {
		return err
	}

	_ = task.ScheduleRefreshApps()
	manager.StartDeviceManager()

	// Check for apps near expiry right at startup so the user doesn't have to
	// keep the app open at 3-6am for the cron to fire.
	go func() {
		time.Sleep(15 * time.Second) // let Fiber + tunneld discovery settle
		task.RefreshExpiringNow()
	}()

	go func() {
		if err := web.Run("127.0.0.1", fiberPort); err != nil {
			log.Error(err.Error())
		}
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", fiberPort), 200*time.Millisecond); err == nil {
			_ = c.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("fiber server did not start in time")
}

func newProxy() http.Handler {
	target, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", fiberPort))
	return httputil.NewSingleHostReverseProxy(target)
}

func main() {
	if err := bootstrapServer(); err != nil {
		fmt.Println("bootstrap error:", err)
		return
	}

	appInstance := NewApp()

	err := wails.Run(&options.App{
		Title:     "iNoaload",
		Width:     820,
		Height:    600,
		MinWidth:  640,
		MinHeight: 480,
		AssetServer: &assetserver.Options{
			Assets:  reactAssets,
			Handler: newProxy(),
		},
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		OnStartup:        appInstance.startup,
		Bind:             []any{appInstance},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			About: &mac.AboutInfo{
				Title:   "iNoaload",
				Message: "Sideload .ipa apps to Apple TV, iPhone and iPad on tvOS/iOS 17+",
			},
		},
	})
	if err != nil {
		fmt.Println("wails error:", err)
	}
}
