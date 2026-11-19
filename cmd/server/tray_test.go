package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"

	"nudge/internal/pair"
)

func TestTrayMenuStatusUsesRealPort(t *testing.T) {
	got := trayMenuModel("18765", nil)
	if got.Status != "Escuchando en el puerto 18765" {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestTrayMenuEmptyState(t *testing.T) {
	got := trayMenuModel("8765", []pair.DeviceInfo{})
	want := trayMenu{Status: "Escuchando en el puerto 8765", Phones: []string{"Ningún teléfono emparejado"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("menu = %#v, want %#v", got, want)
	}
}

func TestTrayMenuPhoneLinesSortedByName(t *testing.T) {
	devices := []pair.DeviceInfo{
		{ID: "a", Name: "pixel", IP: "192.168.1.20"},
		{ID: "b", Name: "Galaxy", IP: "192.168.1.30"},
		{ID: "c", Name: "galaxy", IP: "192.168.1.10"},
	}
	got := trayMenuModel("8765", devices).Phones
	want := []string{
		"galaxy — 192.168.1.10",
		"Galaxy — 192.168.1.30",
		"pixel — 192.168.1.20",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("phones = %q, want %q", got, want)
	}
}

func TestTrayMenuSanitizesAndTruncatesLANText(t *testing.T) {
	devices := []pair.DeviceInfo{
		{ID: "x", Name: "  Evil\u202e\x00 \t phone" + strings.Repeat("n", 60), IP: strings.Repeat("1", 70)},
		{ID: "y", Name: "\u200b\x07", IP: ""},
	}
	got := trayMenuModel("8765", devices).Phones
	wantName := "Evil phone" + strings.Repeat("n", 29) + "…"
	wantIP := strings.Repeat("1", 63) + "…"
	want := []string{
		"(sin nombre) — (IP desconocida)",
		wantName + " — " + wantIP,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("phones = %q, want %q", got, want)
	}
	if n := len([]rune(wantName)); n != maxDialogName {
		t.Fatalf("name runes = %d", n)
	}
}

func TestTrayMenuCapsLongList(t *testing.T) {
	var devices []pair.DeviceInfo
	for i := 0; i < 13; i++ {
		devices = append(devices, pair.DeviceInfo{ID: fmt.Sprint(i), Name: fmt.Sprintf("phone%02d", i), IP: fmt.Sprintf("10.0.0.%d", i)})
	}
	got := trayMenuModel("8765", devices).Phones
	if len(got) != maxTrayPhones+1 {
		t.Fatalf("len = %d: %q", len(got), got)
	}
	if got[0] != "phone00 — 10.0.0.0" || got[9] != "phone09 — 10.0.0.9" {
		t.Fatalf("first/last shown = %q / %q", got[0], got[9])
	}
	if got[10] != "y 3 más" {
		t.Fatalf("overflow line = %q", got[10])
	}
}

func TestTrayMenuExactlyCapHasNoOverflowLine(t *testing.T) {
	var devices []pair.DeviceInfo
	for i := 0; i < maxTrayPhones; i++ {
		devices = append(devices, pair.DeviceInfo{ID: fmt.Sprint(i), Name: fmt.Sprintf("p%02d", i), IP: "1.1.1.1"})
	}
	got := trayMenuModel("8765", devices).Phones
	if len(got) != maxTrayPhones || strings.HasPrefix(got[len(got)-1], "y ") {
		t.Fatalf("phones = %q", got)
	}
}

func TestTrayMenuLabelEscapesAmpersand(t *testing.T) {
	if got := trayMenuLabel("Tom & Jerry — 1.2.3.4"); got != "Tom && Jerry — 1.2.3.4" {
		t.Fatalf("label = %q", got)
	}
}

func TestTrayPairNotice(t *testing.T) {
	title, text := trayPairNotice(pair.RequestInfo{Name: "Pixel\u202e 8\n", IP: "192.168.1.5"})
	if title != "Solicitud de emparejamiento" {
		t.Fatalf("title = %q", title)
	}
	if text != "Pixel 8 quiere emparejarse. Respondé en la ventana de Nudge." {
		t.Fatalf("text = %q", text)
	}
}

func TestTrayPairNoticeTruncatesAndNamesUnknown(t *testing.T) {
	_, text := trayPairNotice(pair.RequestInfo{Name: strings.Repeat("a", 100)})
	if text != strings.Repeat("a", 39)+"… quiere emparejarse. Respondé en la ventana de Nudge." {
		t.Fatalf("text = %q", text)
	}
	_, text = trayPairNotice(pair.RequestInfo{Name: "\x00"})
	if text != "(sin nombre) quiere emparejarse. Respondé en la ventana de Nudge." {
		t.Fatalf("text = %q", text)
	}
}

type fakeTray struct{ calls *[]string }

func (f fakeTray) NotifyPairRequest(req pair.RequestInfo) {
	*f.calls = append(*f.calls, "tray:"+req.RequestID)
}
func (f fakeTray) Close() {}

func TestWithTrayNoticeNotifiesAndKeepsDialogHandler(t *testing.T) {
	var calls []string
	h := withTrayNotice(fakeTray{&calls}, func(req pair.RequestInfo) { calls = append(calls, "dialog:"+req.RequestID) })
	h(pair.RequestInfo{RequestID: "r1"})
	if !reflect.DeepEqual(calls, []string{"tray:r1", "dialog:r1"}) {
		t.Fatalf("calls = %q", calls)
	}
}

func TestNoTrayIsNoOp(t *testing.T) {
	var tr trayHandle = noTray{}
	tr.NotifyPairRequest(pair.RequestInfo{})
	tr.Close()
}

func TestTrayTooltip(t *testing.T) {
	if trayTooltip != "Nudge" {
		t.Fatalf("tooltip = %q", trayTooltip)
	}
}

func TestCopyUTF16TruncatesWithoutSplittingSurrogates(t *testing.T) {
	dst := make([]uint16, 4)
	copyUTF16(dst, "ab😀c")
	// room for 3 units + NUL: "ab" then the 2-unit emoji does not fit.
	if got := string(utf16.Decode(dst[:2])); got != "ab" || dst[2] != 0 || dst[3] != 0 {
		t.Fatalf("dst = %v", dst)
	}
	dst = make([]uint16, 8)
	for i := range dst {
		dst[i] = 'z'
	}
	copyUTF16(dst, "hé")
	if dst[0] != 'h' || dst[1] != 'é' || dst[2] != 0 {
		t.Fatalf("dst = %v", dst)
	}
}
