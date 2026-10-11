package main

import (
	"testing"

	"github.com/ApolloF/syncer/internal/syncthing"
)

func TestLinkedDeviceIsAnIntroducerOnlyBecauseTheUserChoseIt(t *testing.T) {
	d := linkedDevice("ABCDEFG-HIJKLMN", "")
	if !d.Introducer || d.Name != "PC ABCDEFG" {
		t.Errorf("%+v", d)
	}
}

func TestOnlyARealRequestCanBeAccepted(t *testing.T) {
	pending := map[string]syncthing.PendingDevice{"ASKED": {Name: "My Desktop"}}
	if n, err := requestedName(pending, "ASKED"); err != nil || n != "My Desktop" {
		t.Errorf("got %q, %v", n, err)
	}
	if _, err := requestedName(pending, "OTHER"); err == nil {
		t.Error("a device that never asked was accepted")
	}
}
