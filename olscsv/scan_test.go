package olscsv

import (
	"strings"
	"testing"

	"go.nyet.org/xdfkit/model"
)

func TestWriteScanColumns(t *testing.T) {
	m := &model.Model{
		Schema: model.SchemaID,
		Objects: []*model.Object{{
			Key: "t", ID: "T", Address: 0, Rows: 1, Cols: 2, Shape: "1d",
			Data: model.Data{Bits: 8}, Value: model.Value{Conversion: model.Conversion{Factor: 1}},
		}},
	}
	plain, err := Write(m, nil)
	if err != nil || strings.Contains(string(plain), "Value min") {
		t.Fatalf("no image: %v\n%s", err, plain)
	}
	img := []byte{0x10, 0xff}
	with, err := Write(m, img)
	if err != nil {
		t.Fatal(err)
	}
	text := string(with)
	if !strings.Contains(text, "Value min;Value max;Value min*1;Value max*1") {
		t.Fatalf("header:\n%s", text)
	}
	e, _ := model.ScanExtremes(m.Objects[0], img)
	want := []string{fmtFloat(e.Min), fmtFloat(e.Max), e.RawMin, e.RawMax}
	row := strings.Split(strings.Split(strings.TrimRight(text, "\r\n"), "\r\n")[1], ";")
	got := row[len(row)-4:]
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("col %d got %q want %q", i, got[i], want[i])
		}
	}
	back, err := Read(with, "scan.csv")
	if err != nil || back.Objects[0].Extremes != nil || back.Objects[0].ID != "T" {
		t.Fatalf("read ignore: %v %+v", err, back.Objects[0])
	}
}
