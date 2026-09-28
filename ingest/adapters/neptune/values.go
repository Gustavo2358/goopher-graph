package neptune

import (
	"gophergraph/graph"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var integerSyntax = regexp.MustCompile(`^[+-]?[0-9]+$`)
var decimalSyntax = regexp.MustCompile(`^[+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)([eE][+-]?[0-9]+)?$`)
var dateSyntax = regexp.MustCompile(`^([0-9]{4})-([0-9]{2})-([0-9]{2})(T([0-9]{2}):([0-9]{2})(:([0-9]{2})(\.[0-9]{1,9})?)?(Z|[+-][0-9]{2}:?[0-9]{2})?)?$`)

func validDate(s string) bool {
	m := dateSyntax.FindStringSubmatch(s)
	if m == nil {
		return false
	}
	num := func(s string) int { v, _ := strconv.Atoi(s); return v }
	y, mo, d := num(m[1]), num(m[2]), num(m[3])
	if y < 1 || mo < 1 || mo > 12 || d < 1 || d > 31 {
		return false
	}
	dt := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	if dt.Day() != d || int(dt.Month()) != mo {
		return false
	}
	if m[4] != "" {
		if num(m[5]) > 23 || num(m[6]) > 59 || num(m[8]) > 59 {
			return false
		}
		z := strings.ReplaceAll(m[10], ":", "")
		if z != "" && z != "Z" {
			h, mi := num(z[1:3]), num(z[3:])
			if h > 18 || mi > 59 || (h == 18 && mi != 0) {
				return false
			}
		}
	}
	return true
}
func scalar(k graph.ValueKind, s string) (graph.Value, bool, error) {
	switch k {
	case graph.BoolKind:
		if s == "" {
			return graph.Value{}, false, graph.ErrInvalidValue
		}
		return graph.BoolValue(s == "true"), s != "true" && s != "false", nil
	case graph.ByteKind, graph.ShortKind, graph.IntKind, graph.LongKind:
		if !integerSyntax.MatchString(s) {
			return graph.Value{}, false, graph.ErrInvalidValue
		}
		width := map[graph.ValueKind]int{graph.ByteKind: 8, graph.ShortKind: 16, graph.IntKind: 32, graph.LongKind: 64}[k]
		v, e := strconv.ParseInt(s, 10, width)
		if e != nil {
			return graph.Value{}, false, graph.ErrInvalidValue
		}
		x, e := graph.IntegerValue(k, v)
		return x, false, e
	case graph.FloatKind, graph.DoubleKind:
		if s != "NaN" && s != "Infinity" && s != "+Infinity" && s != "-Infinity" && !decimalSyntax.MatchString(s) {
			return graph.Value{}, false, graph.ErrInvalidValue
		}
		width := 64
		if k == graph.FloatKind {
			width = 32
		}
		v, e := strconv.ParseFloat(s, width)
		if e != nil {
			return graph.Value{}, false, graph.ErrInvalidValue
		}
		x, e := graph.DecimalValue(k, v)
		return x, false, e
	case graph.StringKind:
		x, e := graph.TextValue(k, s)
		return x, false, e
	case graph.DateKind, graph.DatetimeKind:
		if !validDate(s) {
			return graph.Value{}, false, graph.ErrInvalidValue
		}
		x, e := graph.TextValue(k, s)
		return x, false, e
	}
	return graph.Value{}, false, graph.ErrInvalidValue
}
func segments(s string, escape bool) []string {
	var result []string
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if escape && s[i] == '\\' && i+1 < len(s) && s[i+1] == ';' {
			b.WriteByte(';')
			i++
			continue
		}
		if s[i] == ';' {
			result = append(result, b.String())
			b.Reset()
		} else {
			b.WriteByte(s[i])
		}
	}
	return append(result, b.String())
}
