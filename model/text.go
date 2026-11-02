package model

import "strings"

// CleanText trims each line and collapses a run of spaces or tabs to one
// space. A newline stays a newline.
func CleanText(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(collapseSpace(line))
	}
	return strings.Join(lines, "\n")
}

func collapseSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	gap := false
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if !gap {
				b.WriteByte(' ')
				gap = true
			}
			continue
		}
		gap = false
		b.WriteRune(r)
	}
	return b.String()
}

// CleanDisplay normalizes display text on m. Descriptions, units, category
// names, and the project name and version are CleanText. A comment is
// trimmed at the ends only, so a column table keeps its internal runs.
func (m *Model) CleanDisplay() {
	if m == nil {
		return
	}
	m.Project.Name = CleanText(m.Project.Name)
	m.Project.Version = CleanText(m.Project.Version)
	for _, c := range m.Categories {
		if c != nil {
			c.Name = CleanText(c.Name)
		}
	}
	for _, o := range m.Objects {
		cleanObject(o)
	}
}

func cleanObject(o *Object) {
	if o == nil {
		return
	}
	o.Description = CleanText(o.Description)
	o.Comment = strings.TrimSpace(o.Comment)
	cleanValue(&o.Value)
	cleanAxis(o.X)
	cleanAxis(o.Y)
	for i := range o.Alternates {
		a := &o.Alternates[i]
		a.Description = CleanText(a.Description)
		cleanValue(&a.Value)
		if a.X != nil {
			cleanValue(a.X)
		}
		if a.Y != nil {
			cleanValue(a.Y)
		}
	}
}

func cleanAxis(a *Axis) {
	if a == nil {
		return
	}
	cleanValue(&a.Value)
}

func cleanValue(v *Value) {
	v.Description = CleanText(v.Description)
	v.Units = CleanText(v.Units)
}
