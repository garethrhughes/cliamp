package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"

	"cliamp/applog"
	"cliamp/external/local"
	"cliamp/internal/playback"
	"cliamp/internal/resume"
	"cliamp/ipc"
	"cliamp/mediactl"
	"cliamp/player"
	"cliamp/playlist"
	"cliamp/ui/model"
)

// runDaemon runs cliamp without a TUI: serves IPC against the shared
// player+playlist, auto-advances tracks, exits on SIGINT/SIGTERM.
func runDaemon(p *player.Player, pl *playlist.Playlist, localProv *local.Provider, autoPlay bool) error {
	fmt.Fprintf(os.Stderr, "cliamp: running headless (socket: %s)\n", ipc.DefaultSocketPath())
	applog.Info("daemon: starting headless mode")

	d := &daemon{
		player:    p,
		playlist:  pl,
		localProv: localProv,
		quit:      make(chan struct{}, 1),
	}

	// Wire MPRIS (Linux) / NowPlaying (macOS) so playerctl and OS media
	// keys see the daemon. mediactl callbacks dispatch back through d.Send.
	svc, mcErr := mediactl.New(func(msg tea.Msg) { d.Send(msg) })
	if mcErr != nil {
		fmt.Fprintf(os.Stderr, "media controls: %v\n", mcErr)
		applog.Warn("daemon: media controls unavailable: %v", mcErr)
	}
	if svc != nil {
		defer svc.Close()
		d.notifier = svc
	}

	if autoPlay && pl.Len() > 0 {
		d.mu.Lock()
		d.playCurrent()
		d.mu.Unlock()
	}

	srv, err := ipc.NewServer(ipc.DefaultSocketPath(), d)
	if err != nil {
		return fmt.Errorf("ipc: %w", err)
	}
	defer srv.Close()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sigCh:
			applog.Info("daemon: signal received, shutting down")
			d.saveResume()
			return nil
		case <-d.quit:
			applog.Info("daemon: quit requested via media control, shutting down")
			d.saveResume()
			return nil
		case <-ticker.C:
			d.tick()
		}
	}
}

// daemon implements ipc.Dispatcher for headless mode. The mutex covers
// playlist state and "what plays next" decisions; the player itself is
// internally thread-safe so blocking I/O (Play, PlayYTDL) runs without it.
type daemon struct {
	mu        sync.Mutex
	player    *player.Player
	playlist  *playlist.Playlist
	localProv *local.Provider
	notifier  playback.Notifier
	quit      chan struct{}
}

func (d *daemon) Send(msg any) {
	d.mu.Lock()
	defer d.mu.Unlock()

	switch m := msg.(type) {
	case ipc.PlayMsg, playback.PlayMsg:
		if d.player.IsPaused() {
			d.player.TogglePause()
		} else if !d.player.IsPlaying() && d.playlist.Len() > 0 {
			d.playCurrent()
		}

	case ipc.PauseMsg, playback.PauseMsg:
		if d.player.IsPlaying() && !d.player.IsPaused() {
			d.player.TogglePause()
		}

	case playback.PlayPauseMsg:
		d.toggle()

	case playback.StopMsg:
		d.player.Stop()

	case playback.NextMsg:
		d.nextTrack()

	case playback.PrevMsg:
		d.prevTrack()

	case playback.QuitMsg:
		select {
		case d.quit <- struct{}{}:
		default:
		}

	case ipc.VolumeMsg:
		d.player.SetVolume(m.DB)

	case playback.SetVolumeMsg:
		d.player.SetVolume(m.VolumeDB)

	case ipc.SeekMsg:
		_ = d.player.Seek(m.Offset)

	case playback.SeekMsg:
		_ = d.player.Seek(m.Offset)

	case playback.SetPositionMsg:
		cur := d.player.Position()
		_ = d.player.Seek(m.Position - cur)

	case ipc.LoadMsg:
		d.handleLoad(m)

	case ipc.QueueMsg:
		d.playlist.Add(playlist.Track{Path: m.Path, Title: m.Path})

	case ipc.ThemeMsg:
		reply(m.Reply, ipc.Response{OK: false, Error: "theme not available in headless mode"})

	case ipc.VisMsg:
		reply(m.Reply, ipc.Response{OK: false, Error: "visualizer not available in headless mode"})

	case ipc.ShuffleMsg:
		d.handleShuffle(m)

	case ipc.RepeatMsg:
		d.handleRepeat(m)

	case ipc.MonoMsg:
		d.handleMono(m)

	case ipc.SpeedMsg:
		d.player.SetSpeed(m.Speed)
		reply(m.Reply, ipc.Response{OK: true, Speed: d.player.Speed()})

	case ipc.EQMsg:
		d.handleEQ(m)

	case ipc.DeviceMsg:
		d.handleDevice(m)

	case ipc.StatusRequestMsg:
		reply(m.Reply, d.statusResponse())
	}
}

// tick advances to the next track when the current one has drained, and
// republishes playback state to the media-control notifier. Daemon mode
// skips gapless preloading; small inter-track gaps are fine.
func (d *daemon) tick() {
	d.mu.Lock()
	if d.player.IsPlaying() && !d.player.IsPaused() && d.player.Drained() {
		d.nextTrack()
	}
	state := d.snapshotState()
	d.mu.Unlock()
	if d.notifier != nil {
		d.notifier.Update(state)
	}
}

// snapshotState builds a playback.State for OS media-control notifiers.
// Caller must hold d.mu.
func (d *daemon) snapshotState() playback.State {
	status := playback.StatusStopped
	if d.player.IsPlaying() {
		if d.player.IsPaused() {
			status = playback.StatusPaused
		} else {
			status = playback.StatusPlaying
		}
	}
	track, _ := d.playlist.Current()
	return playback.State{
		Status: status,
		Track: playback.Track{
			Title:       track.Title,
			Artist:      track.Artist,
			Album:       track.Album,
			Genre:       track.Genre,
			TrackNumber: track.TrackNumber,
			URL:         track.Path,
			ArtURL:      track.ArtURL,
			Duration:    d.player.Duration(),
		},
		VolumeDB: d.player.Volume(),
		Position: d.player.Position(),
		Seekable: d.player.Seekable(),
	}
}

func (d *daemon) playCurrent() {
	track, idx := d.playlist.Current()
	if idx < 0 {
		return
	}
	d.playTrack(track)
}

// playTrack temporarily releases d.mu around the blocking Play call so
// concurrent IPC requests (notably `cliamp status`) don't stall for the
// 1-3s of HTTP/yt-dlp setup. The player itself serializes internally.
// Caller must hold d.mu; the lock is held again on return.
func (d *daemon) playTrack(track playlist.Track) {
	dur := time.Duration(track.DurationSecs) * time.Second
	d.mu.Unlock()
	var err error
	if playlist.IsYTDL(track.Path) {
		err = d.player.PlayYTDL(track.Path, dur)
	} else {
		err = d.player.Play(track.Path, dur)
	}
	d.mu.Lock()
	if err != nil {
		applog.Warn("daemon: play %q: %v", track.Path, err)
	}
}

func (d *daemon) toggle() {
	if !d.player.IsPlaying() {
		if d.playlist.Len() > 0 {
			d.playCurrent()
		}
		return
	}
	d.player.TogglePause()
}

func (d *daemon) nextTrack() {
	track, ok := d.playlist.Next()
	if !ok {
		d.player.Stop()
		return
	}
	d.playTrack(track)
}

func (d *daemon) prevTrack() {
	if d.player.Position() > 3*time.Second {
		track, idx := d.playlist.Current()
		if idx < 0 {
			return
		}
		if d.player.Seekable() {
			_ = d.player.Seek(-d.player.Position())
			return
		}
		d.playTrack(track)
		return
	}
	track, ok := d.playlist.Prev()
	if !ok {
		return
	}
	d.playTrack(track)
}

func (d *daemon) handleLoad(m ipc.LoadMsg) {
	if d.localProv == nil {
		reply(m.Reply, ipc.Response{OK: false, Error: "local provider unavailable"})
		return
	}
	tracks, err := d.localProv.Tracks(m.Playlist)
	if err != nil {
		reply(m.Reply, ipc.Response{OK: false, Error: fmt.Sprintf("playlist %q: %v", m.Playlist, err)})
		return
	}
	d.playlist.Replace(tracks)
	d.playCurrent()
	reply(m.Reply, ipc.Response{OK: true, Playlist: m.Playlist, Total: len(tracks)})
}

func (d *daemon) handleShuffle(m ipc.ShuffleMsg) {
	switch strings.ToLower(m.Name) {
	case "on":
		if !d.playlist.Shuffled() {
			d.playlist.ToggleShuffle()
		}
	case "off":
		if d.playlist.Shuffled() {
			d.playlist.ToggleShuffle()
		}
	default:
		d.playlist.ToggleShuffle()
	}
	shuffled := d.playlist.Shuffled()
	reply(m.Reply, ipc.Response{OK: true, Shuffle: &shuffled})
}

func (d *daemon) handleRepeat(m ipc.RepeatMsg) {
	switch strings.ToLower(m.Name) {
	case "off":
		d.playlist.SetRepeat(playlist.RepeatOff)
	case "all":
		d.playlist.SetRepeat(playlist.RepeatAll)
	case "one":
		d.playlist.SetRepeat(playlist.RepeatOne)
	default:
		d.playlist.CycleRepeat()
	}
	reply(m.Reply, ipc.Response{OK: true, Repeat: d.playlist.Repeat().String()})
}

func (d *daemon) handleMono(m ipc.MonoMsg) {
	switch strings.ToLower(m.Name) {
	case "on":
		if !d.player.Mono() {
			d.player.ToggleMono()
		}
	case "off":
		if d.player.Mono() {
			d.player.ToggleMono()
		}
	default:
		d.player.ToggleMono()
	}
	mono := d.player.Mono()
	reply(m.Reply, ipc.Response{OK: true, Mono: &mono})
}

func (d *daemon) handleEQ(m ipc.EQMsg) {
	if m.Band > 0 || (m.Band == 0 && m.Name == "") {
		d.player.SetEQBand(m.Band, m.Value)
		reply(m.Reply, ipc.Response{OK: true, EQPreset: "Custom"})
		return
	}
	if m.Name == "" {
		reply(m.Reply, ipc.Response{OK: false, Error: "eq requires a preset name or --band"})
		return
	}
	preset, ok := model.EQPresetByName(m.Name)
	if !ok {
		reply(m.Reply, ipc.Response{OK: false, Error: fmt.Sprintf("unknown EQ preset %q", m.Name)})
		return
	}
	for i, gain := range preset.Bands {
		d.player.SetEQBand(i, gain)
	}
	reply(m.Reply, ipc.Response{OK: true, EQPreset: preset.Name})
}

func (d *daemon) handleDevice(m ipc.DeviceMsg) {
	if strings.EqualFold(m.Name, "list") {
		devices, err := player.ListAudioDevices()
		if err != nil {
			reply(m.Reply, ipc.Response{OK: false, Error: fmt.Sprintf("list devices: %v", err)})
			return
		}
		var lines []string
		for _, dev := range devices {
			marker := "  "
			if dev.Active {
				marker = "* "
			}
			lines = append(lines, fmt.Sprintf("%s%s", marker, dev.Name))
		}
		reply(m.Reply, ipc.Response{OK: true, Device: strings.Join(lines, "\n")})
		return
	}
	if err := player.SwitchAudioDevice(m.Name); err != nil {
		reply(m.Reply, ipc.Response{OK: false, Error: fmt.Sprintf("switch device: %v", err)})
		return
	}
	reply(m.Reply, ipc.Response{OK: true, Device: m.Name})
}

func (d *daemon) statusResponse() ipc.Response {
	resp := ipc.Response{OK: true}
	switch {
	case d.player.IsPlaying() && !d.player.IsPaused():
		resp.State = "playing"
	case d.player.IsPaused():
		resp.State = "paused"
	default:
		resp.State = "stopped"
	}
	if cur, _ := d.playlist.Current(); cur.Path != "" {
		resp.Track = &ipc.TrackInfo{
			Title:  cur.Title,
			Artist: cur.Artist,
			Path:   cur.Path,
		}
	}
	pos, dur := d.player.PositionAndDuration()
	resp.Position = pos.Seconds()
	resp.Duration = dur.Seconds()
	resp.Volume = d.player.Volume()
	resp.Index = d.playlist.Index()
	resp.Total = d.playlist.Len()
	shuffled := d.playlist.Shuffled()
	resp.Shuffle = &shuffled
	resp.Repeat = d.playlist.Repeat().String()
	mono := d.player.Mono()
	resp.Mono = &mono
	resp.Speed = d.player.Speed()
	return resp
}

func (d *daemon) saveResume() {
	d.mu.Lock()
	defer d.mu.Unlock()
	track, idx := d.playlist.Current()
	if idx < 0 || track.Path == "" {
		return
	}
	pos := int(d.player.Position().Seconds())
	if pos <= 0 {
		return
	}
	resume.Save(track.Path, pos, "")
}

func reply(ch chan ipc.Response, resp ipc.Response) {
	if ch != nil {
		ch <- resp
	}
}
