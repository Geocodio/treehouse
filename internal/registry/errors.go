package registry

import "errors"

// ErrPoolExhausted is returned by Claim when the configured port pool has
// no block left that is both unclaimed and (when lsof is available)
// free of listening sockets.
var ErrPoolExhausted = errors.New("registry: port pool exhausted")
