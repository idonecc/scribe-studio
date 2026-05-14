// SPDX-License-Identifier: GPL-3.0-or-later
package scribe

import "github.com/autogame-17/scribe-studio/backend/scribe/logbus"

// LogEntry re-exports logbus.Entry under the scribe namespace so the
// Wails TypeScript binding generator surfaces it where the rest of
// the App methods live.
type LogEntry = logbus.Entry

// ListLogs returns up to `limit` most-recent log entries (0 or
// negative ⇒ entire buffer). The Logs page calls this on mount,
// then subscribes to the "log:entry" Wails event for live tail.
func (a *App) ListLogs(limit int) []LogEntry {
	return logbus.List(limit)
}

// ClearLogs empties the in-memory ring buffer. Persisted artifacts
// (e.g. wx_channel's on-disk core.log) are untouched — this only
// resets the UI's tail.
func (a *App) ClearLogs() {
	logbus.Clear()
}
