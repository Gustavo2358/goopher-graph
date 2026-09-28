package neptune

import (
	"errors"
	"gophergraph/graph"
	"gophergraph/ingest/ports"
	"regexp"
	"strings"
	"unicode"
)

type column struct {
	name        string
	kind        graph.ValueKind
	cardinality ports.Cardinality
	array       bool
	reserved    bool
}

var typeSyntax = regexp.MustCompile(`(?i)^(bool|boolean|byte|short|int|long|float|double|string|date|datetime)(\((single|set)\))?(\[\])?$`)
var kinds = map[string]graph.ValueKind{"bool": graph.BoolKind, "boolean": graph.BoolKind, "byte": graph.ByteKind, "short": graph.ShortKind, "int": graph.IntKind, "long": graph.LongKind, "float": graph.FloatKind, "double": graph.DoubleKind, "string": graph.StringKind, "date": graph.DateKind, "datetime": graph.DatetimeKind}

func header(fields []cell, role ports.Role) ([]column, error) {
	cols := make([]column, len(fields))
	seen := map[string]bool{}
	for i, c := range fields {
		s := c.text
		if strings.HasPrefix(s, "~") {
			if s != "~id" && s != "~label" && (role != ports.Edges || (s != "~from" && s != "~to")) {
				return nil, errors.New("reserved column invalid for catalog")
			}
			cols[i] = column{name: s, reserved: true}
		} else {
			var name strings.Builder
			sep := -1
			for j := 0; j < len(s); j++ {
				if s[j] == '\\' && j+1 < len(s) && s[j+1] == ':' {
					name.WriteByte(':')
					j++
					continue
				}
				if s[j] == ':' {
					sep = j
					break
				}
				name.WriteByte(s[j])
			}
			key := name.String()
			if sep < 0 || key == "" || strings.ContainsFunc(key, func(r rune) bool { return unicode.IsSpace(r) || r == ',' }) {
				return nil, errors.New("invalid property name")
			}
			m := typeSyntax.FindStringSubmatch(s[sep+1:])
			if m == nil {
				return nil, errors.New("invalid property type")
			}
			card := ports.Set
			if role == ports.Edges || strings.ToLower(m[3]) == "single" {
				card = ports.Single
			}
			if strings.ToLower(m[3]) == "set" {
				card = ports.Set
			}
			array := m[4] != ""
			if (card == ports.Single && array) || (role == ports.Edges && (card == ports.Set || array)) {
				return nil, errors.New("invalid cardinality")
			}
			cols[i] = column{key, kinds[strings.ToLower(m[1])], card, array, false}
		}
		if seen[cols[i].name] {
			return nil, errors.New("duplicate column")
		}
		seen[cols[i].name] = true
	}
	if !seen["~id"] || (role == ports.Edges && (!seen["~from"] || !seen["~to"])) {
		return nil, errors.New("required column absent")
	}
	return cols, nil
}
