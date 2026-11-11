## Overview of the change %d

The handler reads the request, validates the **payload**, and then calls `store.Save` with the parsed value. Errors from the store are wrapped with context so the caller can tell a *validation* failure from an I/O failure. This paragraph is long enough to wrap across several terminal lines at the narrower widths, which is the common case for streamed replies.

### Steps

- Parse the body with `json.Unmarshal` and reject unknown fields.
- Check the **required** fields: `id`, `name`, and `kind`.
- Persist the record; retry once on `ErrBusy`.
- Return `201 Created` with the new resource location.

```go
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in Input
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.store.Save(r.Context(), in); err != nil {
		http.Error(w, "save failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
```

| Case | Status | Retry |
|------|--------|-------|
| Bad JSON | 400 | no |
| Store busy | 503 | yes |
| Success | 201 | n/a |

> Note: the retry uses a fixed 50 ms delay. Measure before changing it.

1. Run `go test ./...` after each edit.
2. Check the output for **new** failures.

