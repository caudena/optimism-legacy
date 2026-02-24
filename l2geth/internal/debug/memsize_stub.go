package debug

import "net/http"

// memsizeHandler provides a no-op replacement for github.com/fjl/memsize/memsizeui.
// The upstream package relies on runtime internals that break on newer Go versions.
type memsizeHandler struct{}

func (h *memsizeHandler) Add(string, interface{}) {}

func (h *memsizeHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = w.Write([]byte("memsize endpoint disabled"))
}
