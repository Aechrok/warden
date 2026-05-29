// Package docs serves OpenAPI 3.0 specifications and a Swagger UI for each
// Warden API surface (internal, public, SCIM).
package docs

import (
	_ "embed"
	"fmt"
	"net/http"
)

//go:embed internal.json
var internalSpec []byte

//go:embed public.json
var publicSpec []byte

//go:embed scim.json
var scimSpec []byte

// Handler returns an http.Handler that serves the Swagger UI at the root path
// and the raw OpenAPI JSON at /openapi.json, relative to the mount prefix.
//
// specURL must be the absolute URL path where openapi.json is served, e.g.
// "/api/v1/internal/docs/openapi.json".
func handler(spec []byte, specURL, title string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_, _ = w.Write(spec)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, swaggerHTML, title, specURL)
	})
	return mux
}

// InternalHandler serves the internal API docs. Mount at /api/v1/internal/docs/.
func InternalHandler() http.Handler {
	return handler(internalSpec, "/api/v1/internal/docs/openapi.json", "Warden Internal API")
}

// PublicHandler serves the public API docs. Mount at /api/v1/public/docs/.
func PublicHandler() http.Handler {
	return handler(publicSpec, "/api/v1/public/docs/openapi.json", "Warden Public API")
}

// SCIMHandler serves the SCIM API docs. Mount at /scim/v2/docs/.
func SCIMHandler() http.Handler {
	return handler(scimSpec, "/scim/v2/docs/openapi.json", "Warden SCIM 2.0 API")
}

const swaggerHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"/>
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>
  SwaggerUIBundle({
    url: "%s",
    dom_id: "#swagger-ui",
    presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
    layout: "BaseLayout",
    deepLinking: true,
    defaultModelsExpandDepth: 1,
    defaultModelExpandDepth: 1,
  });
</script>
</body>
</html>
`
