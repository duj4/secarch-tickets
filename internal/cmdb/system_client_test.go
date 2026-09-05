package cmdb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestResolveDepartmentsBatchesAndMatchesByLabel(t *testing.T) {
	var mu sync.Mutex
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("schemaName") != systemSchemaName || query.Get("objectType") != systemObjectType {
			t.Errorf("unexpected object scope: %s", r.URL.RawQuery)
		}
		if query.Get("attrsInclude") != systemDepartmentAttr || query.Get("childType") != "false" {
			t.Errorf("unexpected attribute/control query: %s", r.URL.RawQuery)
		}
		mu.Lock()
		requestCount++
		mu.Unlock()

		objects := make([]systemObject, 0)
		iql := query.Get("iql")
		if strings.Contains(iql, "IACV-1") {
			objects = append(objects, systemObject{Label: "System One", Attrs: rawAttrs(t, "Department A")})
		}
		if strings.Contains(iql, "IACV-2") {
			objects = append(objects, systemObject{Label: "System Two", Attrs: rawAttrs(t, "Department B")})
		}
		if strings.Contains(iql, "IACV-3") {
			objects = append(objects, systemObject{Label: "System Three", Attrs: rawAttrs(t, "Department C")})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(systemObjectsResponse{
			Meta: systemObjectsMeta{Page: 1, TotalPages: 1}, Objects: objects,
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		cfg: Config{
			ObjectsAPIURL: server.URL, ObjectBatchSize: 2, ObjectMaxConcurrency: 2,
		},
	}
	resolved, err := client.ResolveDepartments(t.Context(), []SystemReference{
		{Key: "IACV-1", Name: "System One"},
		{Key: "IACV-2", Name: "System Two"},
		{Key: "IACV-3", Name: "System Three"},
		{Key: "IACV-1", Name: "System One"},
	})
	if err != nil {
		t.Fatalf("ResolveDepartments: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("request count = %d, want 2", requestCount)
	}
	for key, want := range map[string]string{
		"IACV-1": "Department A", "IACV-2": "Department B", "IACV-3": "Department C",
	} {
		if got := resolved[key]; got != want {
			t.Errorf("department for %s = %q, want %q", key, got, want)
		}
	}
}

func rawAttrs(t *testing.T, department string) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(department)
	if err != nil {
		t.Fatalf("marshal department: %v", err)
	}
	return map[string]json.RawMessage{systemDepartmentAttr: raw}
}

func TestObjectAttributeStringSupportsObjectValue(t *testing.T) {
	value, err := objectAttributeString(json.RawMessage(`{"label":"Department A","key":"DEP-1"}`))
	if err != nil {
		t.Fatalf("objectAttributeString: %v", err)
	}
	if value != "Department A" {
		t.Fatalf("value = %q, want Department A", value)
	}
}
