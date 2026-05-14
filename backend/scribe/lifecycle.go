// SPDX-License-Identifier: GPL-3.0-or-later
package scribe

import (
	"github.com/autogame-17/scribe-studio/backend/scribe/logbus"
	"wx_channel/pkg/sphkit"
)

// ProxyStatus is the shape returned to the React frontend. It maps 1:1 to
// sphkit.Status but is redeclared here so the Wails TypeScript generator
// places it in the sph package (the frontend imports it as sph.ProxyStatus).
type ProxyStatus struct {
	Running         bool   `json:"running"`
	InterceptorAddr string `json:"interceptorAddr"`
	APIAddr         string `json:"apiAddr"`
	LastError       string `json:"lastError,omitempty"`
}

// StartProxy boots the embedded MITM + API server pair. The kit instance
// is lazily created on first Start so the app window opens instantly and we
// only pay the config-loading cost when the user actually asks to start.
func (a *App) StartProxy() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.kit == nil {
		kit, err := sphkit.New(BuildVersion, BuildMode)
		if err != nil {
			logbus.Error("proxy", "init: %v", err)
			return err
		}
		a.kit = kit
	}
	if err := a.kit.Start(); err != nil {
		logbus.Error("proxy", "start: %v", err)
		return err
	}
	s := a.kit.Status()
	logbus.Info("proxy", "started — interceptor %s, api %s", s.InterceptorAddr, s.APIAddr)
	return nil
}

// StopProxy gracefully shuts the proxy down. Safe to call when not running.
func (a *App) StopProxy() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.kit == nil {
		return nil
	}
	if err := a.kit.Stop(); err != nil {
		logbus.Error("proxy", "stop: %v", err)
		return err
	}
	logbus.Info("proxy", "stopped")
	return nil
}

// GetProxyStatus is what the dashboard polls and also what we emit via
// runtime.EventsEmit("proxy:status", …) when state changes.
func (a *App) GetProxyStatus() ProxyStatus {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.kit == nil {
		return ProxyStatus{}
	}
	s := a.kit.Status()
	return ProxyStatus(s)
}
