package wasmquery

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

// Wazero caps linear memory, but currently exposes no table resource limit.
// This narrow binary admission step accepts Go's zero-or-one funcref table and
// supplies a maximum when omitted. Wazero remains the full WASM validator; no
// instructions, query source or expression language are parsed here.
func limitTable(wasm []byte, max uint32) ([]byte, error) {
	if len(wasm) < 8 || !bytes.Equal(wasm[:8], []byte{0, 97, 115, 109, 1, 0, 0, 0}) {
		return nil, ErrABI
	}
	for offset := 8; offset < len(wasm); {
		start := offset
		id := wasm[offset]
		offset++
		size, n, err := readU32(wasm[offset:])
		if err != nil {
			return nil, err
		}
		offset += n
		if uint64(size) > uint64(len(wasm)-offset) {
			return nil, ErrABI
		}
		end := offset + int(size)
		if id != 4 {
			offset = end
			continue
		}
		table := wasm[offset:end]
		count, n, err := readU32(table)
		if err != nil {
			return nil, err
		}
		table = table[n:]
		if count == 0 {
			if len(table) != 0 {
				return nil, ErrABI
			}
			offset = end
			continue
		}
		if count != 1 {
			return nil, fmt.Errorf("%w: only one function table supported", ErrLimit)
		}
		if len(table) < 2 || table[0] != 0x70 || table[1] > 1 {
			return nil, fmt.Errorf("%w: unsupported table type", ErrABI)
		}
		flags := table[1]
		table = table[2:]
		min, n, err := readU32(table)
		if err != nil {
			return nil, err
		}
		table = table[n:]
		if min > max {
			return nil, fmt.Errorf("%w: function table minimum", ErrLimit)
		}
		if flags == 1 {
			declared, n, err := readU32(table)
			if err != nil {
				return nil, err
			}
			table = table[n:]
			if declared > max {
				return nil, fmt.Errorf("%w: function table maximum", ErrLimit)
			}
			if declared < min || len(table) != 0 {
				return nil, ErrABI
			}
			offset = end
			continue
		}
		if len(table) != 0 {
			return nil, ErrABI
		}
		bounded := []byte{1, 0x70, 1}
		bounded = binary.AppendUvarint(bounded, uint64(min))
		bounded = binary.AppendUvarint(bounded, uint64(max))
		out := make([]byte, 0, len(wasm)+6)
		out = append(out, wasm[:start]...)
		out = append(out, 4)
		out = binary.AppendUvarint(out, uint64(len(bounded)))
		out = append(out, bounded...)
		out = append(out, wasm[end:]...)
		return out, nil
	}
	return wasm, nil
}
func readU32(b []byte) (uint32, int, error) {
	v, n := binary.Uvarint(b)
	if n < 1 || n > 5 || v > math.MaxUint32 {
		return 0, 0, ErrABI
	}
	return uint32(v), n, nil
}
