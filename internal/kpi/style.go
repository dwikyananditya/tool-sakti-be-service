package kpi

import "github.com/xuri/excelize/v2"

type sheetStyle struct {
	headerRGB string
	widths    []float64
	numberCol map[int]bool // zero-based columns formatted as 0.00
	zebra     bool
}

var thinBorder = []excelize.Border{
	{Type: "top", Color: "D0D7DE", Style: 1},
	{Type: "bottom", Color: "D0D7DE", Style: 1},
	{Type: "left", Color: "D0D7DE", Style: 1},
	{Type: "right", Color: "D0D7DE", Style: 1},
}

func writeSheet(f *excelize.File, sheet string, headers []any, rows [][]any, st sheetStyle) {
	cols := len(headers)

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{st.headerRGB}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    thinBorder,
	})
	bodyPlain, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
		Border:    thinBorder,
	})
	bodyZebra, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
		Border:    thinBorder,
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"F7FAFC"}},
	})
	numPlain, _ := f.NewStyle(&excelize.Style{
		Alignment:    &excelize.Alignment{Vertical: "top"},
		Border:       thinBorder,
		CustomNumFmt: strPtr("0.00"),
	})
	numZebra, _ := f.NewStyle(&excelize.Style{
		Alignment:    &excelize.Alignment{Vertical: "top"},
		Border:       thinBorder,
		Fill:         excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"F7FAFC"}},
		CustomNumFmt: strPtr("0.00"),
	})

	_ = f.SetSheetRow(sheet, "A1", &headers)
	for i, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		r := row
		_ = f.SetSheetRow(sheet, cell, &r)
	}

	for i, w := range st.widths {
		name, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sheet, name, name, w)
	}

	last, _ := excelize.CoordinatesToCellName(cols, 1)
	_ = f.SetCellStyle(sheet, "A1", last, headerStyle)

	// Zebra fill is per row, so style row by row, then override number cells.
	for i := range rows {
		rowNum := i + 2
		body := bodyPlain
		num := numPlain
		if st.zebra && (rowNum%2 == 0) {
			body = bodyZebra
			num = numZebra
		}
		start, _ := excelize.CoordinatesToCellName(1, rowNum)
		end, _ := excelize.CoordinatesToCellName(cols, rowNum)
		_ = f.SetCellStyle(sheet, start, end, body)
		for c := range st.numberCol {
			cell, _ := excelize.CoordinatesToCellName(c+1, rowNum)
			_ = f.SetCellStyle(sheet, cell, cell, num)
		}
	}

	if cols > 0 {
		topLeft := "A1"
		bottomRight, _ := excelize.CoordinatesToCellName(cols, len(rows)+1)
		_ = f.AutoFilter(sheet, topLeft+":"+bottomRight, nil)
	}
}

func strPtr(s string) *string { return &s }
