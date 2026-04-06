//go:build !linux

// Package mpris provides a stub implementation for non-Linux platforms
// where D-Bus is not available.
package mpris

import (
	"math"

	"cliamp/internal/control"
	"cliamp/mediactl"
)

// Shared message types (aliases so existing code using mpris.NextMsg still works).
type (
	PlayPauseMsg = control.ToggleMsg
	NextMsg      = control.NextMsg
	PrevMsg      = control.PrevMsg
	StopMsg      = control.StopMsg
)

// MPRIS-specific message types.
type (
	QuitMsg        struct{}
	SeekMsg        struct{ Offset int64 }   // microseconds (relative)
	SetPositionMsg = control.SetPositionMsg // microseconds (absolute)
	SetVolumeMsg   struct{ Volume float64 } // linear 0.0–1.0
)

// TrackInfo is an alias for mediactl.TrackInfo so existing code using
// mpris.TrackInfo continues to compile.
type TrackInfo = mediactl.TrackInfo

// Service is a no-op stub on non-Linux platforms.
type Service struct{}

// New returns nil on non-Linux platforms (no D-Bus available).
func New(send func(interface{})) (*Service, error) {
	return nil, nil
}

// Update is a no-op on non-Linux platforms.
func (s *Service) Update(status string, track TrackInfo, volumeDB float64, positionUs int64, canSeek bool) {
}

// LinearToDb converts a 0.0–1.0 linear volume to dB (range [-30, +6]).
// This must match the Linux implementation in mpris.go.
func LinearToDb(v float64) float64 {
	if v <= 0 {
		return -30
	}
	if v >= 1 {
		return 6
	}
	db := 20*math.Log10(v) + 6
	if db < -30 {
		return -30
	}
	return db
}

// EmitSeeked is a no-op on non-Linux platforms.
func (s *Service) EmitSeeked(positionUs int64) {}

// Close is a no-op on non-Linux platforms.
func (s *Service) Close() {}
