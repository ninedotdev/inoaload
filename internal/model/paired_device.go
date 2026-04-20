package model

import (
	"time"

	"gorm.io/gorm"
)

// PairedDevice tracks devices that have successfully completed a pairing flow.
// For legacy idevicepair pairs this is redundant (usbmuxd already knows), but
// for tvOS 17+ RemoteXPC pairs there is no system-level registry we can query,
// so we record them ourselves here.
type PairedDevice struct {
	gorm.Model
	Name         string `gorm:"uniqueIndex:idx_paired_name_class"`
	DeviceClass  string `gorm:"uniqueIndex:idx_paired_name_class"`
	UDID         string
	IP           string
	PairType     string // "legacy" | "remote"
	LastPairedAt time.Time
}
