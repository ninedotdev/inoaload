package web

import (
	"fmt"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/bitxeno/atvloadly/internal/app"
	"github.com/bitxeno/atvloadly/internal/ipa"
	"github.com/bitxeno/atvloadly/internal/log"
	"github.com/bitxeno/atvloadly/internal/manager"
	"github.com/bitxeno/atvloadly/internal/model"
	"github.com/bitxeno/atvloadly/internal/service"
	"github.com/bitxeno/atvloadly/internal/task"
	"github.com/bitxeno/atvloadly/internal/utils"
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

func route(fi *fiber.App) {
	fi.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	fi.Get("/ws/pair", websocket.New(service.HandlePairMessage))
	fi.Get("/ws/install", websocket.New(service.HandleInstallMessage))

	fi.Get("/apps/:id/icon", func(c *fiber.Ctx) error {
		id := utils.MustParseInt(c.Params("id"))
		t, err := service.GetApp(uint(id))
		if err != nil {
			return c.Status(http.StatusNotFound).SendString(err.Error())
		}
		if t.Icon != "" {
			return c.Status(http.StatusOK).SendFile(t.Icon, false)
		}
		return c.Status(http.StatusNotFound).SendString("")
	})

	api := fi.Group("/api")

	api.Get("/pair/list", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(apiSuccess(manager.ListPairedDevices()))
	})

	api.Post("/pair/delete", func(c *fiber.Ctx) error {
		var body struct {
			Name        string `json:"name"`
			DeviceClass string `json:"device_class"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(http.StatusOK).JSON(apiError(err.Error()))
		}
		if body.Name == "" {
			return c.Status(http.StatusOK).JSON(apiError("name is required"))
		}
		if err := manager.DeletePairedDevice(body.Name, body.DeviceClass); err != nil {
			return c.Status(http.StatusOK).JSON(apiError(err.Error()))
		}
		return c.Status(http.StatusOK).JSON(apiSuccess(true))
	})

	api.Get("/account/teams", func(c *fiber.Ctx) error {
		email := c.Query("email")
		if email == "" {
			return c.Status(http.StatusOK).JSON(apiError("email query param required"))
		}
		teams, err := manager.ListAppleIDTeams(c.Context(), email)
		if err != nil {
			return c.Status(http.StatusOK).JSON(apiError(err.Error()))
		}
		return c.Status(http.StatusOK).JSON(apiSuccess(teams))
	})

	api.Get("/tunneld/status", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(apiSuccess(map[string]any{
			"running": manager.IsTunneldRunning(),
			"host":    "127.0.0.1",
			"port":    49151,
		}))
	})
	api.Post("/tunneld/start", func(c *fiber.Ctx) error {
		if err := manager.StartTunneldWithAdmin(); err != nil {
			return c.Status(http.StatusOK).JSON(apiError(err.Error()))
		}
		return c.Status(http.StatusOK).JSON(apiSuccess(true))
	})

	api.Get("/devices", func(c *fiber.Ctx) error {
		manager.ReloadDevices()
		devices, err := manager.GetDevices()
		if err != nil {
			return c.Status(http.StatusOK).JSON(apiError(err.Error()))
		}
		return c.Status(http.StatusOK).JSON(apiSuccess(devices))
	})

	api.Get("/scan/wireless", func(c *fiber.Ctx) error {
		timeout := 2
		if timeoutStr := c.Query("timeout"); timeoutStr != "" {
			if t := utils.MustParseInt(timeoutStr); t > 0 {
				timeout = t
			}
		}
		devices, err := manager.ScanWirelessDevices(c.Context(), time.Duration(timeout)*time.Second)
		if err != nil {
			return c.Status(http.StatusOK).JSON(apiError(err.Error()))
		}
		return c.Status(http.StatusOK).JSON(apiSuccess(devices))
	})

	api.Post("/upload", func(c *fiber.Ctx) error {
		form, _ := c.MultipartForm()
		files := form.File["files"]

		result := []model.IpaFile{}
		saveDir := filepath.Join(app.Config.Server.DataDir, "tmp")
		if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
			return c.Status(http.StatusOK).JSON(apiError("failed to create directory :" + saveDir))
		}
		for _, file := range files {
			timestamp := time.Now().UnixMicro()
			name := service.GetValidName(utils.FileNameWithoutExt(file.Filename))
			dstName := fmt.Sprintf("%s_%d%s", name, timestamp, filepath.Ext(file.Filename))
			dst := filepath.Join(saveDir, dstName)

			if err := c.SaveFile(file, dst); err != nil {
				return c.Status(http.StatusOK).JSON(apiError(err.Error()))
			}

			ipaFile := model.IpaFile{
				Name: file.Filename,
				Path: dst,
			}

			info, err := ipa.ParseFile(dst)
			if err != nil {
				return c.Status(http.StatusOK).JSON(apiError(err.Error()))
			}

			ipaFile.Name = info.Name()
			ipaFile.BundleIdentifier = info.Identifier()
			ipaFile.Version = info.Version()

			if info.Icon() != nil {
				iconName := fmt.Sprintf("%s_%d%s", name, timestamp, ".png")
				iconDst := filepath.Join(saveDir, iconName)
				out, err := os.Create(iconDst)
				if err == nil {
					defer out.Close()
					if err := png.Encode(out, info.Icon()); err == nil {
						ipaFile.Icon = iconDst
					}
				}
			}

			result = append(result, ipaFile)
		}

		return c.Status(http.StatusOK).JSON(apiSuccess(result))
	})

	api.Get("/apps", func(c *fiber.Ctx) error {
		apps, err := service.GetAppList()
		if err != nil {
			return c.Status(http.StatusOK).JSON(apiError(err.Error()))
		}
		return c.Status(http.StatusOK).JSON(apiSuccess(apps))
	})

	api.Post("/apps/:id/delete", func(c *fiber.Ctx) error {
		id := utils.MustParseInt(c.Params("id"))
		ok, err := service.DeleteApp(uint(id))
		if err != nil {
			return c.Status(http.StatusOK).JSON(apiError(err.Error()))
		}
		return c.Status(http.StatusOK).JSON(apiSuccess(ok))
	})

	api.Post("/apps/:id/reinstall", func(c *fiber.Ctx) error {
		id := utils.MustParseInt(c.Params("id"))
		t, err := service.GetApp(uint(id))
		if err != nil {
			return c.Status(http.StatusOK).JSON(apiError(err.Error()))
		}
		if t.BundleIdentifier != "" && manager.IsRemoteUDID(t.UDID) {
			if err := manager.UninstallRemoteApp(c.Context(), t.UDID, t.Device, t.BundleIdentifier); err != nil {
				log.Err(err).Msg("uninstall before reinstall failed; continuing anyway")
			}
		}
		task.RefreshApp(*t)
		return c.Status(http.StatusOK).JSON(apiSuccess(true))
	})

	api.Post("/apps/cleanup", func(c *fiber.Ctx) error {
		n, err := service.CleanupDuplicateApps()
		if err != nil {
			return c.Status(http.StatusOK).JSON(apiError(err.Error()))
		}
		return c.Status(http.StatusOK).JSON(apiSuccess(map[string]int{"deleted": n}))
	})

	api.Post("/apps/:id/toggle", func(c *fiber.Ctx) error {
		id := utils.MustParseInt(c.Params("id"))
		app, err := service.ToggleAppEnabled(uint(id))
		if err != nil {
			return c.Status(http.StatusOK).JSON(apiError(err.Error()))
		}
		return c.Status(http.StatusOK).JSON(apiSuccess(app))
	})

	api.Post("/apps/:id/refresh", func(c *fiber.Ctx) error {
		id := utils.MustParseInt(c.Params("id"))
		t, err := service.GetApp(uint(id))
		if err != nil {
			return c.Status(http.StatusOK).JSON(apiError(err.Error()))
		}
		task.RefreshApp(*t)
		return c.Status(http.StatusOK).JSON(apiSuccess(true))
	})
}
