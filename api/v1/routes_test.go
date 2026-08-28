package v1

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/wI2L/fizz"
)

func TestTaskResultOpenAPIResponseContent(t *testing.T) {
	r := fizz.NewFromEngine(gin.New())
	InitRoutes(r)
	if errs := r.Errors(); len(errs) > 0 {
		t.Fatalf("route generation errors: %v", errs)
	}
	tests := []struct {
		path, mediaType, schemaType, format string
	}{
		{"/v1/inference_tasks/{client_task_id}/images/{index}", "image/png", "string", "binary"},
		{"/v1/inference_tasks/{client_task_id}/llm", "application/json", "object", ""},
	}
	for _, tt := range tests {
		path := r.Generator().API().Paths[tt.path]
		if path == nil || path.GET == nil {
			t.Fatalf("missing GET operation for %s", tt.path)
		}
		response := path.GET.Responses["200"]
		if response == nil {
			t.Fatalf("missing 200 response for %s", tt.path)
		}
		media := response.Content[tt.mediaType]
		if media == nil || media.MediaType == nil || media.Schema == nil || media.Schema.Schema == nil {
			t.Fatalf("missing %s content schema for %s", tt.mediaType, tt.path)
		}
		if media.Schema.Type != tt.schemaType || media.Schema.Format != tt.format {
			t.Fatalf("schema for %s = %s/%s", tt.path, media.Schema.Type, media.Schema.Format)
		}
	}
}

func TestOrdinaryImageGenerationRouteIsNotRegistered(t *testing.T) {
	r := fizz.NewFromEngine(gin.New())
	InitRoutes(r)
	if errs := r.Errors(); len(errs) > 0 {
		t.Fatalf("route generation errors: %v", errs)
	}
	path := r.Generator().API().Paths["/v1/images"]
	if path != nil && path.POST != nil {
		t.Fatal("POST /v1/images is still registered")
	}
	modelsPath := r.Generator().API().Paths["/v1/images/models"]
	if modelsPath == nil || modelsPath.POST == nil {
		t.Fatal("POST /v1/images/models is not registered")
	}
}
