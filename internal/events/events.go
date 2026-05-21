package events

import "time"

// Event is a single camera-side detection event. Fields mirror what the camera
// reports natively; consumers map them to higher-level meaning.
type Event struct {
	Time   time.Time      `json:"time"`
	Code   string         `json:"code"`   // e.g. VideoMotion, SmartMotionHuman, CrossLineDetection
	Action string         `json:"action"` // Start | Stop | Pulse
	Index  int            `json:"index"`  // channel/region index reported by the camera
	Data   map[string]any `json:"data,omitempty"`
}

// DefaultCodes is the set of Dahua/Amcrest event codes we subscribe to by default.
// VideoMotion is the basic pixel-difference detector; the rest are AI-confirmed.
var DefaultCodes = []string{
	"VideoMotion",
	"SmartMotionHuman",
	"SmartMotionVehicle",
	"CrossLineDetection",
	"CrossRegionDetection",
}
