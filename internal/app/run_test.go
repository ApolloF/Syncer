package app

import (
	"testing"

	"github.com/ApolloF/syncer/internal/meta"
)

func TestApplyOptions(t *testing.T) {
	defer func(v, id, secret string, icon []byte) {
		version, gdriveClientID, gdriveClientSecret, trayIcon = v, id, secret, icon
		meta.AppVersion = v
	}(version, gdriveClientID, gdriveClientSecret, trayIcon)

	apply(Options{Version: "v9.9.9", GdriveClientID: "id", GdriveClientSecret: "secret", TrayIcon: []byte{1}})
	if version != "v9.9.9" || meta.AppVersion != "v9.9.9" {
		t.Errorf("version = %q, meta = %q", version, meta.AppVersion)
	}
	if c := gdriveConfig(); c.ClientID != "id" || c.ClientSecret != "secret" {
		t.Errorf("gdrive config = %+v", c)
	}
	if len(trayIcon) != 1 {
		t.Errorf("tray icon not handed in")
	}
}
