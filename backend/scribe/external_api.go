// SPDX-License-Identifier: GPL-3.0-or-later
package scribe

import (
	"errors"

	"github.com/autogame-17/scribe-studio/backend/scribe/external"
)

// Re-export so the Wails binding generator emits the types under the
// `scribe` namespace alongside the wx_channel TaskSummary.
type ExternalTask = external.Task
type ExternalProbe = external.ProbeResult
type ExternalFormat = external.Format
type ExternalAddRequest = external.AddRequest

// ResolveURL probes a URL with yt-dlp and returns enough metadata
// (title, available resolutions, subtitle languages) to populate the
// "add URL" modal. Cookie file is optional — pass an empty string
// for public videos.
func (a *App) ResolveURL(url, cookieFile string) (ExternalProbe, error) {
	mgr := a.externalManager()
	if mgr == nil {
		return ExternalProbe{}, errors.New("external manager not ready")
	}
	return mgr.Probe(url, cookieFile)
}

// AddExternalURL creates a new external download task and kicks off
// the yt-dlp subprocess in the background. The created Task is
// returned so the UI can render the row immediately; subsequent
// updates arrive via the "external:task" Wails event.
func (a *App) AddExternalURL(req ExternalAddRequest) (ExternalTask, error) {
	mgr := a.externalManager()
	if mgr == nil {
		return ExternalTask{}, errors.New("external manager not ready")
	}
	// Make sure the manager uses the currently-configured download
	// dir — the user may have changed Settings since boot.
	mgr.SetDownloadDir(a.resolveDownloadDir())
	return mgr.AddURL(req)
}

// ListExternalTasks returns every persisted external download,
// newest first. The Downloads page merges this with ListTasks
// (wx_channel) into a single list.
func (a *App) ListExternalTasks() []ExternalTask {
	mgr := a.externalManager()
	if mgr == nil {
		return nil
	}
	return mgr.List()
}

// RetryExternal re-runs a finished/errored/canceled external task.
func (a *App) RetryExternal(id string) error {
	mgr := a.externalManager()
	if mgr == nil {
		return errors.New("external manager not ready")
	}
	return mgr.Retry(id)
}

// CancelExternal interrupts an in-flight external download.
func (a *App) CancelExternal(id string) error {
	mgr := a.externalManager()
	if mgr == nil {
		return errors.New("external manager not ready")
	}
	return mgr.Cancel(id)
}

// RemoveExternal deletes the task from persisted state. Files on
// disk are not touched (the user may still want them); the wx_channel
// equivalent doesn't have a delete-from-history button either, but
// since external tasks can pile up from one-off tries we expose one.
func (a *App) RemoveExternal(id string) error {
	mgr := a.externalManager()
	if mgr == nil {
		return errors.New("external manager not ready")
	}
	return mgr.Remove(id)
}

func (a *App) externalManager() *external.Manager {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.external
}
