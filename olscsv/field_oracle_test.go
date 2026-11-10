package olscsv

import (
	"encoding/csv"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"go.nyet.org/xdfkit/kp"
)

// Compares WinOLS CSV columns to KP fields on the local 839 pair.
// Prints per-field match rates; fails only if IdName/StartAddr join breaks.
func TestLocalCSVFieldOracle(t *testing.T) {
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
	text, err := decodeLatin1(cb)
	if err != nil {
		t.Fatal(err)
	}
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = ';'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	idx := indexHeader(rows[0])
	f, err := kp.Parse(kb)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]*kp.Map{}
	for _, m := range f.Project.Maps {
		if m.ID != "" {
			byID[m.ID] = m
		}
	}
	folderName := map[int32]string{}
	for _, fd := range f.Project.Folders {
		folderName[fd.ID] = fd.Name
	}

	type ctr struct{ n, ok int }
	c := map[string]*ctr{}
	hit := func(name string, good bool) {
		x := c[name]
		if x == nil {
			x = &ctr{}
			c[name] = x
		}
		x.n++
		if good {
			x.ok++
		}
	}
	eqf := func(a, b float64) bool {
		if a == b {
			return true
		}
		return math.Abs(a-b) <= 1e-9*(1+math.Abs(a)+math.Abs(b))
	}
	parseSig := func(s string) (int32, bool) {
		s = strings.TrimSpace(s)
		s = strings.TrimPrefix(strings.ToLower(s), "0x")
		if s == "" || s == "-" {
			return 0, false
		}
		v, err := strconv.ParseInt(s, 0, 32)
		return int32(v), err == nil
	}

	joined := 0
	for _, row := range rows[1:] {
		id := cell(row, idx, "IdName")
		m, ok := byID[id]
		if !ok || id == "" {
			continue
		}
		joined++
		get := func(name string) string { return cell(row, idx, name) }

		addr, err := parseAddr(get("Fieldvalues.StartAddr"))
		hit("Fieldvalues.StartAddr", err == nil && addr == m.Start)
		hit("Name", get("Name") == m.Name)
		hit("Comment", get("Comment") == m.Comment)
		hit("FolderName", get("FolderName") == folderName[m.FolderID])
		hit("Type", csvOrg(get("Type")) == m.Type)
		hit("ViewMode", csvViewMode(get("ViewMode")) == m.ViewMode)
		hit("RWin", csvRWin(get("RWin")) == m.RWin)
		hit("DataOrg", csvDataOrg(get("DataOrg")) == m.DataOrg)
		hit("bReciprocal", (get("bReciprocal") == "1") == m.Reciprocal)
		hit("bSigned", (get("bSigned") == "1") == m.Signed)
		hit("bDelta", (get("bDelta") == "1") == m.Difference)
		hit("bPercent", (get("bPercent") == "1") == m.Percent)
		hit("Columns", atoiDefault(get("Columns"), 1) == int(m.Cols))
		hit("Rows", atoiDefault(get("Rows"), 1) == int(m.Rows))
		hit("Radix", radixBase(get("Radix")) == int(m.Radix))
		hit("Precision", atoiDefault(get("Precision"), 0) == int(m.Precision))
		hit("Marker", (get("Marker") == "1") == (m.Marked != 0))
		hit("Fieldvalues.Name", get("Fieldvalues.Name") == m.Value.Description)
		hit("Fieldvalues.Unit", dashEmpty(get("Fieldvalues.Unit")) == m.Value.Units)
		hit("Fieldvalues.Factor", eqf(atofDefault(get("Fieldvalues.Factor"), 1), m.Value.Factor))
		hit("Fieldvalues.Offset", eqf(atofDefault(get("Fieldvalues.Offset"), 0), m.Value.Offset))

		// Value range for the map width slot.
		w := m.DataOrg.Width()
		slot := map[int]int{1: 0, 2: 1, 4: 2}[w]
		if slot >= 0 && w > 0 {
			lo, _ := strconv.ParseInt(get("ValueRangeMin"), 10, 64)
			hi, _ := strconv.ParseInt(get("ValueRangeMax"), 10, 64)
			hit("ValueRange", lo == m.Range[slot][0] && hi == m.Range[slot][1])
		}

		cmpAxis := func(prefix string, a *kp.Axis, defined bool) {
			if a == nil {
				return
			}
			// Always compare description/units/factor for defined axes;
			// ordinal slots still carry residue in KP.
			hit(prefix+".Name", get(prefix+".Name") == a.Value.Description)
			hit(prefix+".Unit", dashEmpty(get(prefix+".Unit")) == a.Value.Units)
			hit(prefix+".Factor", eqf(atofDefault(get(prefix+".Factor"), 1), a.Value.Factor))
			hit(prefix+".Offset", eqf(atofDefault(get(prefix+".Offset"), 0), a.Value.Offset))
			hit(prefix+".Precision", atoiDefault(get(prefix+".Precision"), 0) == int(a.Precision))
			hit(prefix+".Radix", radixBase(get(prefix+".Radix")) == int(a.Radix))
			hit(prefix+".bReciprocal", (get(prefix+".bReciprocal") == "1") == a.Reciprocal)
			hit(prefix+".bSigned", (get(prefix+".bSigned") == "1") == a.Signed)
			hit(prefix+".bBackwards", (get(prefix+".bBackwards") == "1") == a.Backwards)
			hit(prefix+".DataSrc", csvDataSrc(get(prefix+".DataSrc")) == a.DataSource)
			hit(prefix+".DataHeader", atoiDefault(get(prefix+".DataHeader"), 0) == int(a.DataHeader))
			hit(prefix+".DataOrg", csvDataOrg(get(prefix+".DataOrg")) == a.DataOrg)
			if sig, ok := parseSig(get(prefix + ".SignatureByte")); ok {
				hit(prefix+".SignatureByte", sig == a.SignatureByte)
			}
			if a.DataSource.FromImage() {
				ad, err := parseAddr(get(prefix + ".DataAddr"))
				hit(prefix+".DataAddr", err == nil && ad == a.Addr)
			}
			_ = defined
		}
		cmpAxis("AxisX", m.X, m.X != nil && m.X.Defined)
		cmpAxis("AxisY", m.Y, m.Y != nil && m.Y.Defined)

		// Columns with no known KP home: report non-zero rate only.
		hit("bOriginal_nonzero", get("bOriginal") == "0")
		hit("bOriginalValues_nonzero", get("bOriginalValues") == "0")
		hit("SkipBytes_zero", get("SkipBytes") == "0" || get("SkipBytes") == "")
		hit("LineSkipBytes_zero", get("LineSkipBytes") == "0" || get("LineSkipBytes") == "")
		hit("ValueRangeLock_zero", get("ValueRangeLock") == "0" || get("ValueRangeLock") == "")
	}

	if joined < 4000 {
		t.Fatalf("joined %d", joined)
	}
	// Confirmed with value variance on this pack.
	for _, name := range []string{"RWin", "ViewMode", "bSigned"} {
		x := c[name]
		if x == nil || x.n == 0 || x.ok != x.n {
			t.Fatalf("%s: %v (want 100%% match)", name, x)
		}
	}
	t.Logf("joined %d maps", joined)
	order := []string{
		"Fieldvalues.StartAddr", "Name", "Comment", "FolderName", "Type", "ViewMode", "RWin",
		"DataOrg", "bReciprocal", "bSigned", "bDelta", "bPercent", "Columns", "Rows", "Radix",
		"Precision", "Marker", "Fieldvalues.Name", "Fieldvalues.Unit", "Fieldvalues.Factor",
		"Fieldvalues.Offset", "ValueRange",
		"AxisX.Name", "AxisX.Unit", "AxisX.Factor", "AxisX.Offset", "AxisX.Precision", "AxisX.Radix",
		"AxisX.bReciprocal", "AxisX.bSigned", "AxisX.bBackwards", "AxisX.DataSrc", "AxisX.DataHeader",
		"AxisX.DataOrg", "AxisX.SignatureByte", "AxisX.DataAddr",
		"AxisY.Name", "AxisY.Unit", "AxisY.Factor", "AxisY.Offset", "AxisY.Precision", "AxisY.Radix",
		"AxisY.bReciprocal", "AxisY.bSigned", "AxisY.bBackwards", "AxisY.DataSrc", "AxisY.DataHeader",
		"AxisY.DataOrg", "AxisY.SignatureByte", "AxisY.DataAddr",
		"bOriginal_nonzero", "bOriginalValues_nonzero", "SkipBytes_zero", "LineSkipBytes_zero", "ValueRangeLock_zero",
	}
	for _, name := range order {
		x := c[name]
		if x == nil || x.n == 0 {
			t.Logf("%-28s  (no samples)", name)
			continue
		}
		t.Logf("%-28s  %6d/%-6d  %.2f%%", name, x.ok, x.n, 100*float64(x.ok)/float64(x.n))
	}
}

func csvOrg(s string) kp.Org {
	switch s {
	case "eEinzel":
		return kp.OrgSingle
	case "eEindim":
		return kp.OrgOneD
	case "eZweidim":
		return kp.OrgTwoD
	case "eZweidimInverse", "e2dInverse", "eZweidimInv":
		return kp.OrgTwoDInv
	}
	return 0
}

func csvViewMode(s string) int32 {
	switch s {
	case "eViewText":
		return 1
	case "eView2d", "eView2D":
		return 2
	case "eView3d", "eView3D":
		return 3
	}
	return -1
}

func csvRWin(s string) int32 {
	switch s {
	case "eRightWinNone", "eNone", "":
		return 0
	case "eHex":
		return 1
	case "eBars":
		return 2
	case "eHexBars", "eHexAndBars":
		return 3
	}
	return -1
}

func csvDataOrg(s string) kp.DataOrg {
	switch s {
	case "eByte", "eBits":
		return 1
	case "eHiLo":
		return 2
	case "eLoHi":
		return 3
	case "eHiLoHiLo":
		return 4
	case "eLoHiLoHi":
		return 5
	case "eFloatHiLo":
		return 6
	case "eFloatLoHi":
		return 7
	}
	return 0
}

func csvDataSrc(s string) kp.DataSource {
	switch s {
	case "eDataSrcNone", "None", "":
		return kp.DSOrdinal
	case "eRom", "eEprom", "eEEPROM", "eDataSrcEprom", "eDataSrcRom":
		return kp.DSEeprom
	case "eRomAdd", "eEpromAdd", "eDataSrcEpromAdd":
		return kp.DSEepromAdd
	case "eRomSub", "eEpromSub", "eEpromSubtract", "eDataSrcEpromSub":
		return kp.DSEepromSubtract
	case "eUserdef", "eFreeEditable", "eDataSrcFreeEditable":
		return kp.DSFreeEditable
	case "eRomBackwards", "eEpromBackwards", "eDataSrcEpromBackwards":
		return kp.DSEepromBackwards
	}
	return 255
}
