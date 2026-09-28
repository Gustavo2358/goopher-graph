// Verifica premissas da stdlib utilizadas pela especificação.
// NÃO implementa o adapter Neptune nem a engine.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func require(ok bool, message string) {
	if !ok {
		panic(message)
	}
}

func main() {
	r := csv.NewReader(strings.NewReader(",\"\"\n"))
	row, err := r.Read()
	require(err == nil && len(row) == 2 && row[0] == "" && row[1] == "", "csv: blank and quoted empty")

	r = csv.NewReader(strings.NewReader("\"line1\r\nline2\"\r\n"))
	row, err = r.Read()
	require(err == nil && row[0] == "line1\nline2", "csv: CRLF normalization")

	var b bytes.Buffer
	w := csv.NewWriter(&b)
	require(w.Write([]string{""}) == nil, "csv writer failed")
	w.Flush()
	require(w.Error() == nil && b.String() == "\n", "csv: empty single column")

	r = csv.NewReader(strings.NewReader("a,b\n1\n2,3\n"))
	_, err = r.Read()
	require(err == nil, "csv header")
	_, err = r.Read()
	require(errors.Is(err, csv.ErrFieldCount), "csv field count")
	row, err = r.Read()
	require(err == nil && len(row) == 2 && row[1] == "3", "csv after field-count error")

	v, err := strconv.ParseFloat("0x1p2", 64)
	require(err == nil && v == 4, "strconv accepts hex floats; adapter must gate syntax")
	yes, err := strconv.ParseBool("TRUE")
	require(err == nil && yes, "strconv bool grammar differs from the local profile")

	var data [8]byte
	binary.LittleEndian.PutUint64(data[:], 0x1020304050607080)
	require(binary.LittleEndian.Uint64(data[:]) == 0x1020304050607080 && data[0] == 0x80, "LE scalar")
	fmt.Println("PASS: 7 stdlib premises; no engine/Neptune adapter executed")
}
