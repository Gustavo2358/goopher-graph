`filtered_v1.wasm.gz` preserves the pre-selection packaged `filtered` version 1
module (gzip with timestamp zero). The compatibility test executes these actual
legacy bytes against the updated host and compares them with version 2. It needs
no Git history or network. `guest` is compiled locally for SDK integration tests.
