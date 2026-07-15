package instance

import (
	"fmt"
	"strings"

	"github.com/geocodio/treehouse/internal/manifest"
)

// RenderedURL is one of the manifest's urls entries with its "{VAR}"
// placeholders substituted for this instance's actual port numbers.
type RenderedURL struct {
	Name string
	URL  string
}

// RenderURLs substitutes every "{ENV_VAR}" placeholder in the manifest's
// urls section with this instance's actual port numbers, for the
// human-facing summary printed after an instance comes up (and returned in
// `create --json`'s "urls" field).
func RenderURLs(urls []manifest.URL, ports map[string]int) []RenderedURL {
	rendered := make([]RenderedURL, 0, len(urls))
	for _, url := range urls {
		rendered = append(rendered, RenderedURL{
			Name: url.Name,
			URL:  substitutePorts(url.URL, ports),
		})
	}

	return rendered
}

func substitutePorts(template string, ports map[string]int) string {
	result := template
	for name, port := range ports {
		placeholder := fmt.Sprintf("{%s}", name)
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%d", port))
	}

	return result
}
