// Package installedwasm owns the immutable deployment registry, not the WASM ABI.
package installedwasm

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"gophergraph/wasmquery"
	"regexp"
	"slices"
)

type Descriptor struct {
	Name, SHA256, Version, Description, ABI string
	MinArgs, MaxArgs                        int
	Parameters                              []string
}
type entry struct {
	descriptor Descriptor
	module     *wasmquery.Module
}
type Registry struct {
	entries map[string]entry
	ordered []Descriptor
	digest  string
}
type Installation struct {
	Name, Version, Description string
	Parameters                 []string
	MinArgs, MaxArgs           int
	Wasm                       []byte
}

var validName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

func Load(ctx context.Context, r *wasmquery.Runtime, installations []Installation) (*Registry, error) {
	reg := &Registry{entries: make(map[string]entry)}
	for _, i := range installations {
		if !validName.MatchString(i.Name) || i.MinArgs < 0 || i.MaxArgs < i.MinArgs || i.MaxArgs > 1024 {
			return nil, fmt.Errorf("invalid installed query descriptor %q", i.Name)
		}
		if _, ok := reg.entries[i.Name]; ok {
			return nil, fmt.Errorf("installed query name collision %q", i.Name)
		}
		m, err := r.Compile(ctx, i.Wasm)
		if err != nil {
			return nil, fmt.Errorf("compile installed query %s: %w", i.Name, err)
		}
		if err = r.Prepare(ctx, m); err != nil {
			return nil, fmt.Errorf("prepare installed query %s: %w", i.Name, err)
		}
		d := Descriptor{i.Name, m.SHA256(), i.Version, i.Description, "gophergraph_v1", i.MinArgs, i.MaxArgs, slices.Clone(i.Parameters)}
		reg.entries[i.Name] = entry{d, m}
		reg.ordered = append(reg.ordered, d)
	}
	slices.SortFunc(reg.ordered, func(a, b Descriptor) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	h := sha256.New()
	var word [8]byte
	number := func(n uint64) { binary.BigEndian.PutUint64(word[:], n); _, _ = h.Write(word[:]) }
	text := func(v string) { number(uint64(len(v))); _, _ = h.Write([]byte(v)) }
	number(uint64(len(reg.ordered)))
	for _, d := range reg.ordered {
		for _, v := range []string{d.Name, d.SHA256, d.Version, d.Description, d.ABI} {
			text(v)
		}
		number(uint64(d.MinArgs))
		number(uint64(d.MaxArgs))
		number(uint64(len(d.Parameters)))
		for _, p := range d.Parameters {
			text(p)
		}
	}
	reg.digest = fmt.Sprintf("%x", h.Sum(nil))
	return reg, nil
}
func Default(ctx context.Context, r *wasmquery.Runtime) (*Registry, error) {
	definitions := []Installation{
		{Name: "shared-targets", Version: "1", Description: "Common reachable nodes and induced edges", MinArgs: 2, MaxArgs: 2, Parameters: []string{"from", "other"}},
		{Name: "between", Version: "1", Description: "Forward/reverse reachability intersection", MinArgs: 2, MaxArgs: 2, Parameters: []string{"from", "to"}},
		{Name: "filtered", Version: "1", Description: "Typed labelled targets, callers and connecting edges", MinArgs: 4, MaxArgs: 4, Parameters: []string{"node", "label", "property", "value"}},
	}
	files := []string{"shared_targets", "between", "filtered"}
	for n := range definitions {
		b, err := assets.ReadFile("assets/" + files[n] + ".wasm")
		if err != nil {
			return nil, err
		}
		definitions[n].Wasm = b
	}
	return Load(ctx, r, definitions)
}
func (r *Registry) Lookup(name string) (Descriptor, *wasmquery.Module, bool) {
	e, ok := r.entries[name]
	d := e.descriptor
	d.Parameters = slices.Clone(d.Parameters)
	return d, e.module, ok
}
func (r *Registry) List() []Descriptor {
	out := slices.Clone(r.ordered)
	for i := range out {
		out[i].Parameters = slices.Clone(out[i].Parameters)
	}
	return out
}
func (r *Registry) SHA256() string { return r.digest }
