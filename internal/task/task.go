package task

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"io"
	stdhttp "net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bitxeno/atvloadly/internal/app"
	"github.com/bitxeno/atvloadly/internal/ipa"
	"github.com/bitxeno/atvloadly/internal/log"
	"github.com/bitxeno/atvloadly/internal/manager"
	"github.com/bitxeno/atvloadly/internal/model"
	"github.com/bitxeno/atvloadly/internal/service"
	"github.com/bitxeno/atvloadly/internal/utils"
	"github.com/robfig/cron/v3"
)

var instance = new()

type Task struct {
	c               *cron.Cron
	InstallingApps  sync.Map
	InstallAppQueue chan TaskItem
	chExitQueue     chan bool
	InvalidAccounts map[string]bool
	// RefreshingDevices prevents concurrent refresh operations for the same device UDID
	RefreshingDevices sync.Map
	// Batch tracking for aggregated notifications
	batchMu      sync.Mutex
	currentBatch *BatchInfo
}

type TaskItem struct {
	App     model.InstalledApp
	Notify  bool
	BatchID string
}

type BatchInfo struct {
	ID           string
	TotalCount   int
	SuccessCount int
	FailedApps   []FailedAppInfo
	Notify       bool
}

type FailedAppInfo struct {
	AppName string
	Account string
	Error   string
}

func new() *Task {
	return &Task{
		InstallAppQueue: make(chan TaskItem, 100),
		chExitQueue:     make(chan bool, 1),
	}
}

func (t *Task) RunSchedule() error {
	if t.c != nil {
		t.Stop()
	}

	t.c = cron.New()
	if _, err := t.c.AddFunc(app.Settings.Task.CrodTime, t.Run); err != nil {
		log.Err(err).Msgf("Failed to start app refresh scheduled task due to incorrect timing format: %s", app.Settings.Task.CrodTime)
		t.c = nil
		return err
	}

	t.Start()

	return nil
}

func (t *Task) Start() {
	if app.Settings.Task.Enabled {
		log.Infof("App refresh scheduled task has started, time: %s", app.Settings.Task.CrodTime)
		t.c.Start()
	} else {
		log.Warn("App refresh scheduled task is disabled.")
	}

	go t.runQueue()
}

func (t *Task) Stop() {
	t.chExitQueue <- true
	<-t.c.Stop().Done()
	t.c = nil
}

func (t *Task) Run() {
	installedApps, err := service.GetEnableAppList()
	if err != nil {
		log.Err(err).Msg("Failed to get the installation list")
		return
	}

	appsNeedRefresh := make([]model.InstalledApp, 0)
	for _, v := range installedApps {
		if !v.NeedRefresh(app.Settings.Task.AdvanceDays) {
			continue
		}

		if v.IsAccountInvalid() {
			log.Warnf("The install account (%s) is invalid, skip refresh app: %s.", v.MaskAccount(), v.IpaName)
			continue
		}

		appsNeedRefresh = append(appsNeedRefresh, v)
	}

	if len(appsNeedRefresh) == 0 {
		log.Info("No apps need to be refreshed.")
		return
	}

	log.Infof("Start executing installation task (%d need refresh)...", len(appsNeedRefresh))
	t.StartInstallApps(appsNeedRefresh, true)
}

func (t *Task) StartInstallApps(apps []model.InstalledApp, notify bool) {
	t.resetInvalidAccounts()

	if len(apps) == 0 {
		return
	}

	// Create a batch for aggregated notification
	batchID := fmt.Sprintf("batch-%d", time.Now().UnixNano())
	t.batchMu.Lock()
	t.currentBatch = &BatchInfo{
		ID:           batchID,
		TotalCount:   len(apps),
		SuccessCount: 0,
		FailedApps:   make([]FailedAppInfo, 0),
		Notify:       notify,
	}
	t.batchMu.Unlock()

	for _, v := range apps {
		t.startInstallAppInternal(v, notify, batchID)
	}
}

func (t *Task) startInstallAppInternal(v model.InstalledApp, notify bool, batchID string) {
	if _, loaded := t.InstallingApps.LoadOrStore(v.ID, v); !loaded {
		select {
		case t.InstallAppQueue <- TaskItem{App: v, Notify: notify, BatchID: batchID}:
		default:
			t.InstallingApps.Delete(v.ID)
			log.Warnf("The install queue is full, skip task: %s", v.IpaName)
		}
	}
}

func (t *Task) runQueue() {
	// Wait for one minute before install at startup to avoid the usbmuxd service not being ready.
	time.Sleep(time.Minute)

	for {
		select {
		case v := <-t.InstallAppQueue:
			t.tryInstallApp(v)
			t.InstallingApps.Delete(v.App.ID)

			// Next execution delayed by 10 seconds.
			time.Sleep(10 * time.Second)
		case <-t.chExitQueue:
			log.Info("Install app queue exit.")
			return
		}
	}
}

func (t *Task) tryInstallApp(item TaskItem) {
	resolvedApp, err := t.resolveIPA(item.App)
	if err != nil {
		log.Err(err).Msgf("Prepare ipa path failed: %s", item.App.IpaName)
		t.handleInstallFailure(item, item.App, err)
		return
	}
	v := *resolvedApp

	log.Infof("Start installing ipa: %s", v.IpaName)
	logger := manager.NewTaskLogger()
	defer manager.CleanInstallTempFiles(v.IpaPath)

	if v.Account == "" || v.UDID == "" {
		logger.Write("account or UDID is empty")
		logger.SaveLog(v.ID)
		t.handleInstallFailure(item, v, fmt.Errorf("account or UDID is empty"))
		return
	}
	if _, ok := t.InvalidAccounts[v.Account]; ok {
		logger.Write(fmt.Sprintf("The install account (%s) is invalid, skip install.", v.MaskAccount()))
		logger.SaveLog(v.ID)
		t.handleInstallFailure(item, v, manager.ErrAccountInvalid)
		return
	}
	if !manager.IsRemoteUDID(v.UDID) {
		err := fmt.Errorf("device %s is not a tvOS/iOS 17+ RemoteXPC target", v.UDID)
		logger.Write("ERROR: " + err.Error())
		logger.SaveLog(v.ID)
		t.handleInstallFailure(item, v, err)
		return
	}

	rm := manager.NewRemoteInstallManager()
	rm.OnOutput(func(line string) { logger.Write(line) })
	defer rm.Close()

	startErr := rm.Start(context.Background(), manager.InstallOptions{
		UDID:             v.UDID,
		Account:          v.Account,
		Password:         v.Password,
		IpaPath:          v.IpaPath,
		RemoveExtensions: v.RemoveExtensions,
		DeviceName:       v.Device,
	})
	if startErr != nil || !logger.IsSuccess() {
		effErr := startErr
		if effErr == nil {
			if logger.IsAccountInvalid() {
				t.InvalidAccounts[v.Account] = true
				effErr = manager.ErrAccountInvalid
			} else {
				effErr = fmt.Errorf("install failed")
			}
		}
		logger.SaveLog(v.ID)
		t.handleInstallFailure(item, v, effErr)
		return
	}

	now := time.Now()
	expirationDate := now.AddDate(0, 0, 7)
	v.RefreshedDate = &now
	v.ExpirationDate = &expirationDate
	v.RefreshedResult = true
	v.RefreshedError = model.RefreshedErrorNone

	if v.ID == 0 {
		savedApp, saveErr := service.SaveApp(v)
		if saveErr != nil {
			log.Err(saveErr).Msgf("Save app failed after installation success: %s", v.IpaName)
			logger.SaveLog(v.ID)
			t.handleInstallFailure(item, v, saveErr)
			return
		}
		v = *savedApp
	} else {
		if updateErr := service.UpdateAppRefreshResult(v); updateErr != nil {
			log.Err(updateErr).Msgf("Update app refresh result failed: %s", v.IpaName)
		}
	}

	logger.SaveLog(v.ID)
	log.Infof("Installing ipa success: %s", v.IpaName)
	t.trackBatchProgress(item, true, nil)
}

func (t *Task) handleInstallFailure(item TaskItem, v model.InstalledApp, err error) {
	log.Err(err).Msgf("Installing ipa failed: %s", v.IpaName)
	v.RefreshedResult = false
	if errors.Is(err, manager.ErrAccountInvalid) {
		v.RefreshedError = model.RefreshedErrorInvalidAccount
	} else {
		v.RefreshedError = model.RefreshedErrorInvalidOther
	}
	if v.ID != 0 {
		_ = service.UpdateAppRefreshResult(v)
	}

	// Track batch progress and send aggregated notification
	t.trackBatchProgress(item, false, err)
}

func (t *Task) resolveIPA(v model.InstalledApp) (*model.InstalledApp, error) {
	// Not new install, return directly to avoid unnecessary download
	if v.ID != 0 {
		return &v, nil
	}

	saveDir := filepath.Join(app.Config.Server.DataDir, "tmp")
	if strings.HasPrefix(v.IpaPath, "http:") || strings.HasPrefix(v.IpaPath, "https:") {
		if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
			return nil, fmt.Errorf("failed to create temporary directory: %w", err)
		}

		tmpFile, err := os.CreateTemp(saveDir, "install_*.ipa")
		if err != nil {
			return nil, fmt.Errorf("failed to create temporary file: %w", err)
		}
		tmpIPAPath := tmpFile.Name()
		_ = tmpFile.Close()

		resp, err := stdhttp.Get(v.IpaPath)
		if err != nil {
			_ = os.Remove(tmpIPAPath)
			return nil, fmt.Errorf("failed to download ipa: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = os.Remove(tmpIPAPath)
			return nil, fmt.Errorf("download failed with status code %d", resp.StatusCode)
		}
		out, err := os.Create(tmpIPAPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create temp ipa: %w", err)
		}
		if _, err := io.Copy(out, resp.Body); err != nil {
			_ = out.Close()
			_ = os.Remove(tmpIPAPath)
			return nil, fmt.Errorf("failed to save ipa: %w", err)
		}
		_ = out.Close()
		v.IpaPath = tmpIPAPath
	}

	info, err := ipa.ParseFile(v.IpaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ipa file: %w", err)
	}

	v.IpaName = info.Name()
	v.BundleIdentifier = info.Identifier()
	v.Version = info.Version()

	// 保存icon
	if info.Icon() != nil {
		timestamp := time.Now().UnixMicro()
		name := service.GetValidName(utils.FileNameWithoutExt(v.IpaPath))
		iconName := fmt.Sprintf("%s_%d%s", name, timestamp, ".png")
		iconDst := filepath.Join(saveDir, iconName)
		out, err := os.Create(iconDst)
		if err == nil {
			defer out.Close()

			if err := png.Encode(out, info.Icon()); err == nil {
				v.Icon = iconDst
			}
		}
	}

	return &v, nil
}

func (t *Task) trackBatchProgress(item TaskItem, success bool, err error) {
	t.batchMu.Lock()
	defer t.batchMu.Unlock()

	if t.currentBatch == nil || t.currentBatch.ID != item.BatchID {
		return
	}

	if success {
		t.currentBatch.SuccessCount++
	} else {
		t.currentBatch.FailedApps = append(t.currentBatch.FailedApps, FailedAppInfo{
			AppName: item.App.IpaName,
			Account: item.App.Account,
			Error:   err.Error(),
		})
	}

	// Check if batch is complete
	completedCount := t.currentBatch.SuccessCount + len(t.currentBatch.FailedApps)
	if completedCount >= t.currentBatch.TotalCount {
		// Batch complete, send aggregated notification
		t.sendBatchNotification(t.currentBatch)
		t.currentBatch = nil
	}
}

func (t *Task) sendBatchNotification(batch *BatchInfo) {
	_ = batch
}

func (t *Task) resetInvalidAccounts() {
	t.InvalidAccounts = make(map[string]bool)
}

func ScheduleRefreshApps() error {
	return instance.RunSchedule()
}

func RefreshApp(v model.InstalledApp) {
	instance.StartInstallApps([]model.InstalledApp{v}, true)
}

// RefreshExpiringNow scans installed apps on startup and queues any that are
// within the advance_days window for refresh. Useful when the app wasn't open
// at the 3-6am cron window.
func RefreshExpiringNow() {
	apps, err := service.GetEnableAppList()
	if err != nil {
		log.Err(err).Msg("RefreshExpiringNow: failed to get app list")
		return
	}
	for _, a := range apps {
		if !a.NeedRefresh(app.Settings.Task.AdvanceDays) {
			continue
		}
		if a.IsAccountInvalid() {
			continue
		}
		log.Infof("RefreshExpiringNow: queueing %s (expires %v)", a.IpaName, a.ExpirationDate)
		instance.StartInstallApps([]model.InstalledApp{a}, false)
	}
}
