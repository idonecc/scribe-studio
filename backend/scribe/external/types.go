// SPDX-License-Identifier: GPL-3.0-or-later
// Package external integrates third-party URL downloaders (currently
// yt-dlp) so the user can transcribe videos from YouTube, Bilibili,
// Twitter, and any other site yt-dlp supports — alongside the
// WeChat Channels MITM pipeline the app was built around.
//
// The contract this package upholds with the rest of the codebase:
//
//   - It owns its own state (external.json under StateDir) and never
//     touches gopeed.db. wx_channel tasks live in BoltDB; external
//     tasks live here. The Wails layer merges both streams for the UI.
//   - When a download finishes, the manager calls back into a
//     "completed" callback that the pipeline subscribes to so the
//     transcribe stage fires automatically.
//   - All long-running work is cancellable. The user can cancel a
//     running download from the UI without leaving zombie processes.
package external

// Stage names mirror the high-level state machine the UI renders.
// Kept as named constants rather than an int so the JSON state file
// is human-readable.
const (
	StatusPending     = "pending"     // accepted, not yet probed
	StatusProbing     = "probing"     // running yt-dlp -J for metadata
	StatusDownloading = "downloading" // yt-dlp main download in flight
	StatusMerging     = "merging"     // ffmpeg post-merge step
	StatusDone        = "done"        // file on disk, ready for transcribe
	StatusError       = "error"
	StatusCanceled    = "canceled"
)

// Task is the per-URL record persisted in external.json and broadcast
// to the UI via Wails events.
type Task struct {
	ID        string `json:"id"`        // "ext_<hex>"
	URL       string `json:"url"`       // user-supplied URL
	Title     string `json:"title"`     // populated after probe
	Site      string `json:"site"`      // yt-dlp extractor key (e.g. "youtube", "BiliBili")
	Duration  float64 `json:"duration"` // seconds; 0 if unknown
	Thumbnail string `json:"thumbnail,omitempty"`

	// Format / options the user picked when adding the task.
	Format     string   `json:"format"`               // yt-dlp -f selector (e.g. "bv*[height<=1080]+ba/b")
	FormatHint string   `json:"formatHint,omitempty"` // human label, e.g. "1080p"
	CookieFile string   `json:"cookieFile,omitempty"`
	SubLangs   []string `json:"subLangs,omitempty"` // subtitles to fetch (yt-dlp --sub-langs)

	// Runtime state.
	Status      string  `json:"status"`
	Progress    float64 `json:"progress"`    // 0..1
	ProgressMsg string  `json:"progressMsg,omitempty"`
	Downloaded  int64   `json:"downloaded"`  // bytes
	TotalBytes  int64   `json:"totalBytes"`  // bytes (estimate; -1 if unknown)
	Speed       int64   `json:"speed"`       // bytes/sec
	ETA         int     `json:"eta"`         // seconds

	// Final output.
	Path     string `json:"path"`     // download directory
	Filename string `json:"filename"` // final filename after merge/move
	Error    string `json:"error,omitempty"`

	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// VideoPath joins Path + Filename. Empty string if either is missing.
func (t Task) VideoPath() string {
	if t.Path == "" || t.Filename == "" {
		return ""
	}
	if t.Path[len(t.Path)-1] == '/' {
		return t.Path + t.Filename
	}
	return t.Path + "/" + t.Filename
}

// ProbeResult is the slimmed-down view of `yt-dlp -J` output that the
// frontend uses to populate the "add URL" modal (title preview +
// resolution dropdown). We don't expose yt-dlp's full schema.
type ProbeResult struct {
	URL       string   `json:"url"`
	Title     string   `json:"title"`
	Site      string   `json:"site"`
	Duration  float64  `json:"duration"`
	Thumbnail string   `json:"thumbnail,omitempty"`
	Uploader  string   `json:"uploader,omitempty"`
	Formats   []Format `json:"formats"`
	SubLangs  []string `json:"subLangs,omitempty"`
}

// Format is the simplified format entry surfaced to the dropdown.
// We collapse yt-dlp's audio/video/combined options into "what the
// user usually wants": a height-keyed list of best combined streams.
type Format struct {
	ID       string `json:"id"`       // pass to yt-dlp -f
	Label    string `json:"label"`    // "1080p · mp4 · ~50 MB"
	Height   int    `json:"height"`
	FileSize int64  `json:"fileSize"` // bytes; 0 if unknown
	Ext      string `json:"ext"`
}

// AddRequest captures everything the UI needs to send to start an
// external download. URL is required; everything else has sensible
// defaults filled in by the manager.
type AddRequest struct {
	URL        string   `json:"url"`
	Format     string   `json:"format,omitempty"`     // empty → "bv*+ba/b"
	FormatHint string   `json:"formatHint,omitempty"` // for display
	CookieFile string   `json:"cookieFile,omitempty"`
	SubLangs   []string `json:"subLangs,omitempty"`
	// If Title is set we use it instead of re-probing (saves a round
	// trip when the UI already called Probe).
	Title    string  `json:"title,omitempty"`
	Site     string  `json:"site,omitempty"`
	Duration float64 `json:"duration,omitempty"`
}
