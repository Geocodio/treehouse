package instance

import (
	"testing"

	"github.com/geocodio/treehouse/internal/manifest"
)

func TestRenderURLs(t *testing.T) {
	urls := []manifest.URL{
		{Name: "App", URL: "http://localhost:{HTTP_PORT}"},
		{Name: "Mail", URL: "http://localhost:{MAIL_UI_PORT}/mail"},
	}
	ports := map[string]int{"HTTP_PORT": 4640, "MAIL_UI_PORT": 4642}

	got := RenderURLs(urls, ports)

	want := []RenderedURL{
		{Name: "App", URL: "http://localhost:4640"},
		{Name: "Mail", URL: "http://localhost:4642/mail"},
	}

	if len(got) != len(want) {
		t.Fatalf("RenderURLs() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("RenderURLs()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
