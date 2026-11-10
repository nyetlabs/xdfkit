package olscsv

import (
	"os"
	"path/filepath"
	"testing"

	"go.nyet.org/xdfkit/kp"
	"go.nyet.org/xdfkit/model"
)

// Local WinOLS CSV + KP pair (skip when absent). Joins by IdName/key and
// checks addresses; also round-trips the CSV through Read/Write.
func TestLocalIncomingCSVJoin(t *testing.T) {
	dir := filepath.Join("..", "testdata", "local", "incoming", "csv")
	csvPath := filepath.Join(dir, "06A906032HJ 0002.csv")
	kpPath := filepath.Join(dir, "06A906032HJ 0002.kp")
	cb, err := os.ReadFile(csvPath)
	if err != nil {
		t.Skip(err)
	}
	kb, err := os.ReadFile(kpPath)
	if err != nil {
		t.Skip(err)
	}
	cm, err := Read(cb, filepath.Base(csvPath))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Write(cm)
	if err != nil {
		t.Fatal(err)
	}
	cm2, err := Read(out, "round.csv")
	if err != nil {
		t.Fatal(err)
	}
	if len(cm2.Objects) != len(cm.Objects) {
		t.Fatalf("csv round trip objects %d -> %d", len(cm.Objects), len(cm2.Objects))
	}

	f, err := kp.Parse(kb)
	if err != nil {
		t.Fatal(err)
	}
	km := model.FromKP(f)
	byID := map[string]*model.Object{}
	for _, o := range km.Objects {
		if o.ID != "" {
			byID[o.ID] = o
		}
	}
	matched := 0
	var addrMismatch int
	for _, o := range cm.Objects {
		if o.ID == "" {
			continue
		}
		k, ok := byID[o.ID]
		if !ok {
			continue
		}
		matched++
		if o.Address != k.Address {
			addrMismatch++
			if addrMismatch <= 5 {
				t.Logf("addr %s csv=%x kp=%x", o.ID, o.Address, k.Address)
			}
		}
	}
	if matched < 4000 {
		t.Fatalf("joined only %d maps (csv %d kp %d)", matched, len(cm.Objects), len(km.Objects))
	}
	if addrMismatch > 0 {
		t.Fatalf("%d address mismatches of %d joined", addrMismatch, matched)
	}
}
