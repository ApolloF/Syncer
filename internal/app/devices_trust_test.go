package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/ApolloF/syncer/internal/store"
	"github.com/ApolloF/syncer/internal/syncthing"
)

func TestLinkedDeviceGetsAPlainNameWhenNoneIsGiven(t *testing.T) {
	if d := linkedDevice("ABCDEFG-HIJKLMN", ""); d.Name != "PC ABCDEFG" {
		t.Errorf("%+v", d)
	}
}

func TestALinkedPCIntroducesNoOne(t *testing.T) {
	if d := linkedDevice("ABCDEFG-HIJKLMN", "Laptop"); d.Introducer || d.AutoAcceptFolders {
		t.Errorf("%+v", d)
	}
}

func TestJoinListNamesWhoALinkedPCIsLinkedTo(t *testing.T) {
	names := map[string]string{"ASKER": "Asker", "VIA": "SYNCER-A"}
	intro := map[string]store.Introduced{"THIRDPC": {Name: "SYNCER-C", Via: "VIA"}, "GONEVIA": {Name: "", Via: "OLDPCID1"}}
	got := joinList([]string{"ASKER", "GONEVIA", "STRANGER", "THIRDPC"}, names, intro)
	want := []JoinView{{ID: "ASKER", Name: "Asker"}, {ID: "GONEVIA", Name: "GONEVIA", Via: "OLDPCID"}, {ID: "THIRDPC", Name: "SYNCER-C", Via: "SYNCER-A"}}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestOnlyAPCThatAsksCanBeAllowedOrIgnored(t *testing.T) {
	const asked, turnedDown = "MFZWI3D-BONSGYC-YLTMRWG-C43ENR5-QXGZDMM-FZWI3DP-BONSGYY-LTMRWAD", "MFZWI3D-BONSGYC-YLTMRWG-C43ENR5-QXGZDMM-FZWI3DP-BONSGYY-LTMRWAE"
	waiting, ignored := []string{asked}, []string{turnedDown}
	if id, err := joinRequest(strings.ToLower(asked), waiting, ignored, false); err != nil || id != asked {
		t.Errorf("got %q, %v", id, err)
	}
	if _, err := joinRequest(turnedDown, waiting, ignored, true); err != nil {
		t.Errorf("an ignored PC can't be allowed: %v", err)
	}
	if _, err := joinRequest(turnedDown, waiting, ignored, false); err == nil {
		t.Error("an ignored PC was ignored again")
	}
	if _, err := joinRequest("MFZWI3D-BONSGYC-YLTMRWG-C43ENR5-QXGZDMM-FZWI3DP-BONSGYY-LTMRWAF", waiting, ignored, true); err == nil {
		t.Error("a PC that never asked was allowed")
	}
	if _, err := joinRequest("nonsense", waiting, ignored, true); err == nil {
		t.Error("a bad ID was accepted")
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
