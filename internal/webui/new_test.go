//go:build !webui

package webui

import "testing"

func TestNewWithoutTheTag(t *testing.T) {
	t.Parallel()

	h, embedded, err := New()
	if err != nil || embedded || h == nil {
		t.Fatalf("New() = %v, %v, %v; want the notice handler, not embedded", h, embedded, err)
	}
}
