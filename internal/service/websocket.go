package service

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitxeno/atvloadly/internal/log"
	"github.com/bitxeno/atvloadly/internal/manager"
	"github.com/bitxeno/atvloadly/internal/model"
	"github.com/gofiber/contrib/websocket"
)

type installBackend interface {
	Write(p []byte)
	Close()
}

func HandleInstallMessage(c *websocket.Conn) {
	websocketMgr := manager.NewWebsocketManager(c)
	defer websocketMgr.Cancel()

	var backend installBackend
	defer func() {
		if backend != nil {
			backend.Close()
		}
	}()

	for {
		msg, err := websocketMgr.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err) || websocket.IsCloseError(err) {
				return
			}
			log.Err(err).Msg("Read websocket message error: ")
			return
		}

		switch msg.Type {
		case model.MessageTypeInstall:
			var v model.InstalledApp
			if err := json.Unmarshal([]byte(msg.Data), &v); err != nil {
				_ = c.WriteMessage(websocket.TextMessage, []byte("ERROR: "+err.Error()))
				continue
			}
			if v.Account == "" {
				_ = c.WriteMessage(websocket.TextMessage, []byte("account is empty"))
				continue
			}
			// tvOS 17+ RemoteXPC identifiers rotate — the backend resolves the
			// device as long as a name is provided and the tunnel exists.
			if v.UDID == "" && v.Device == "" {
				_ = c.WriteMessage(websocket.TextMessage, []byte("UDID or device name is required"))
				continue
			}

			rm := manager.NewRemoteInstallManager()
			appRecord := v
			rm.OnOutput(func(line string) {
				websocketMgr.WriteMessage(line)
				if strings.HasPrefix(strings.ToLower(line), "installation succeeded") {
					SaveRemoteInstalledApp(&appRecord)
				}
			})
			backend = rm
			go func() {
				opts := manager.InstallOptions{
					UDID:             v.UDID,
					Account:          v.Account,
					Password:         v.Password,
					IpaPath:          v.IpaPath,
					RemoveExtensions: v.RemoveExtensions,
					TeamID:           v.TeamID,
					DeviceName:       v.Device,
				}
				if err := rm.Start(websocketMgr.Context(), opts); err != nil {
					websocketMgr.WriteMessage("Installation Failed!")
				}
			}()
		case model.MessageType2FA:
			if backend != nil {
				backend.Write([]byte(msg.Data + "\n"))
			}
		default:
			_ = c.WriteMessage(websocket.TextMessage, []byte("ERROR: invalid message type"))
			continue
		}
	}
}

// SaveRemoteInstalledApp persists the record of a successful RemoteXPC install
// so the background refresh task can re-sign it before the 7-day cert expiry.
func SaveRemoteInstalledApp(v *model.InstalledApp) {
	now := time.Now()
	expires := now.AddDate(0, 0, 7)
	v.InstalledDate = &now
	v.RefreshedDate = &now
	v.ExpirationDate = &expires
	v.RefreshedResult = true
	v.Enabled = true
	if v.DeviceClass == "" {
		v.DeviceClass = "AppleTV"
	}
	if v.IpaName == "" && v.IpaPath != "" {
		v.IpaName = filepath.Base(v.IpaPath)
	}
	_, _ = SaveApp(*v)
}

type pairBackend interface {
	Write(p []byte)
	Close()
}

func HandlePairMessage(c *websocket.Conn) {
	websocketMgr := manager.NewWebsocketManager(c)
	defer websocketMgr.Cancel()

	var backend pairBackend
	defer func() {
		if backend != nil {
			backend.Close()
		}
	}()

	for {
		msg, err := websocketMgr.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err) || websocket.IsCloseError(err) {
				return
			}
			log.Err(err).Msg("Read websocket message error: ")
			return
		}

		switch msg.Type {
		case model.MessageTypePair:
			if backend != nil {
				backend.Close()
			}
			// Data format expected: "remote:<name>" for tvOS/iOS 17+ RemoteXPC.
			name, isRemote := strings.CutPrefix(msg.Data, "remote:")
			if !isRemote {
				websocketMgr.WriteMessage("ERROR: only RemoteXPC (tvOS/iOS 17+) pairing is supported")
				continue
			}
			rm := manager.NewRemotePairManager()
			rm.OnOutput(func(line string) {
				websocketMgr.WriteMessage(line)
				if strings.HasPrefix(strings.ToLower(line), "success:") {
					manager.SaveRemotePaired(name, "AppleTV", "")
				}
			})
			backend = rm
			go func() {
				if err := rm.Start(websocketMgr.Context(), name); err != nil {
					websocketMgr.WriteMessage(fmt.Sprintf("ERROR: %s", err.Error()))
				}
			}()
		case model.MessageTypePairConfirm:
			if backend != nil {
				backend.Write([]byte(msg.Data + "\n"))
			}
		default:
			_ = c.WriteMessage(websocket.TextMessage, []byte("ERROR: invalid message type"))
			continue
		}
	}
}
