package wasmquery

import (
	"context"
	"fmt"
	"github.com/tetratelabs/wazero"
	"slices"
)

// Prepare validates imports against the installed host exports and instantiates
// once at startup without invoking the WASI _start query. A binary start section
// may run, under the usual time/memory sandbox, with no graph capabilities.
// This catches link/initialization failures before an installed module is READY.
func (r *Runtime) Prepare(ctx context.Context, m *Module) error {
	ctx, done, err := r.begin(ctx, r.limits.CompileTimeout)
	if err != nil {
		return err
	}
	defer done()
	if m == nil || m.owner != r {
		return ErrABI
	}
	for _, f := range m.compiled.ImportedFunctions() {
		mod, name, _ := f.Import()
		host := r.rt.Module(mod)
		if host == nil {
			return fmt.Errorf("%w: missing host module %s", ErrABI, mod)
		}
		fn := host.ExportedFunctionDefinitions()[name]
		if fn == nil || !slices.Equal(fn.ParamTypes(), f.ParamTypes()) || !slices.Equal(fn.ResultTypes(), f.ResultTypes()) {
			return fmt.Errorf("%w: unresolved import %s.%s", ErrABI, mod, name)
		}
	}
	instance, err := r.rt.InstantiateModule(ctx, m.compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions())
	if instance != nil {
		closeErr := instance.Close(context.Background())
		if err == nil {
			err = closeErr
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
