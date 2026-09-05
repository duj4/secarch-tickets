package cmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	systemSchemaName     = "IT Asset"
	systemObjectType     = "System"
	systemDepartmentAttr = "Technology Owning Super Department"
)

type systemObjectsResponse struct {
	Meta    systemObjectsMeta `json:"meta"`
	Objects []systemObject    `json:"objects"`
}

type systemObjectsMeta struct {
	LastPage   int `json:"lastPage"`
	Page       int `json:"page"`
	TotalPages int `json:"totalPages"`
}

type systemObject struct {
	Key       string                     `json:"key"`
	ObjectKey string                     `json:"objectKey"`
	Label     string                     `json:"label"`
	Attrs     map[string]json.RawMessage `json:"attrs"`
}

// ResolveDepartments resolves each unique System key in bounded batches.
func (c *Client) ResolveDepartments(ctx context.Context, references []SystemReference) (map[string]string, error) {
	unique := make(map[string]SystemReference, len(references))
	for _, reference := range references {
		reference.Key = strings.ToUpper(strings.TrimSpace(reference.Key))
		reference.Name = strings.TrimSpace(reference.Name)
		if reference.Key == "" {
			continue
		}
		if !issueKeyPattern.MatchString(reference.Key) {
			return nil, fmt.Errorf("invalid System key %q", reference.Key)
		}
		if existing, ok := unique[reference.Key]; !ok || existing.Name == "" {
			unique[reference.Key] = reference
		}
	}
	if len(unique) == 0 {
		return map[string]string{}, nil
	}

	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	resolved := make(map[string]string, len(keys))
	groupCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	semaphore := make(chan struct{}, c.cfg.ObjectMaxConcurrency)
	var waitGroup sync.WaitGroup
	var resolvedMu sync.Mutex
	var firstError error
	var errorOnce sync.Once

	for start := 0; start < len(keys); start += c.cfg.ObjectBatchSize {
		end := min(start+c.cfg.ObjectBatchSize, len(keys))
		batchKeys := append([]string(nil), keys[start:end]...)
		batchReferences := make(map[string]SystemReference, len(batchKeys))
		for _, key := range batchKeys {
			batchReferences[key] = unique[key]
		}

		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-groupCtx.Done():
				return
			}
			batchResolved, err := c.resolveDepartmentBatch(groupCtx, batchKeys, batchReferences)
			if err != nil {
				errorOnce.Do(func() {
					firstError = err
					cancel()
				})
				return
			}
			resolvedMu.Lock()
			for key, department := range batchResolved {
				resolved[key] = department
			}
			resolvedMu.Unlock()
		}()
	}

	waitGroup.Wait()
	if firstError != nil {
		return nil, firstError
	}
	return resolved, nil
}

func (c *Client) resolveDepartmentBatch(ctx context.Context, keys []string, references map[string]SystemReference) (map[string]string, error) {
	iql := buildSystemKeysIQL(keys)
	objects := make([]systemObject, 0, len(keys))

	for page := 1; ; page++ {
		result, err := c.querySystemObjectsPage(ctx, iql, page, len(keys))
		if err != nil {
			return nil, fmt.Errorf("resolve System batch beginning with %s: %w", keys[0], err)
		}
		objects = append(objects, result.Objects...)
		lastPage := result.Meta.TotalPages
		if result.Meta.LastPage > lastPage {
			lastPage = result.Meta.LastPage
		}
		if lastPage <= page {
			break
		}
	}

	byName := make(map[string][]string, len(references))
	for key, reference := range references {
		if reference.Name != "" {
			name := strings.ToLower(reference.Name)
			byName[name] = append(byName[name], key)
		}
	}

	resolved := make(map[string]string, len(keys))
	for _, object := range objects {
		key := identifySystemObject(object, references, byName)
		if key == "" {
			return nil, fmt.Errorf("cannot match CMDB System object %q to a requested key", object.Label)
		}
		department, err := objectAttributeString(object.Attrs[systemDepartmentAttr])
		if err != nil {
			return nil, fmt.Errorf("read %s for %s: %w", systemDepartmentAttr, key, err)
		}
		if department == "" {
			return nil, fmt.Errorf("CMDB System %s has no %s", key, systemDepartmentAttr)
		}
		resolved[key] = department
	}

	for _, key := range keys {
		if resolved[key] == "" {
			return nil, fmt.Errorf("CMDB objects API did not return System %s", key)
		}
	}
	return resolved, nil
}

func (c *Client) querySystemObjectsPage(ctx context.Context, iql string, page, pageSize int) (*systemObjectsResponse, error) {
	requestURL, err := c.buildSystemObjectsRequestURL(iql, page, pageSize)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create CMDB objects request: %w", err)
	}
	req.Header.Set("accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query CMDB objects API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("CMDB objects API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result systemObjectsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode CMDB objects response: %w", err)
	}
	return &result, nil
}

func (c *Client) buildSystemObjectsRequestURL(iql string, page, pageSize int) (string, error) {
	baseURL, err := url.Parse(c.cfg.ObjectsAPIURL)
	if err != nil {
		return "", fmt.Errorf("parse objects_api_url %q: %w", c.cfg.ObjectsAPIURL, err)
	}
	params := url.Values{}
	params.Set("schemaName", systemSchemaName)
	params.Set("objectType", systemObjectType)
	params.Set("iql", iql)
	params.Set("childType", "false")
	params.Add("attrsInclude", systemDepartmentAttr)
	params.Set("page", strconv.Itoa(page))
	params.Set("pageSize", strconv.Itoa(pageSize))
	baseURL.RawQuery = params.Encode()
	return baseURL.String(), nil
}

func buildSystemKeysIQL(keys []string) string {
	quoted := make([]string, 0, len(keys))
	for _, key := range keys {
		quoted = append(quoted, `"`+key+`"`)
	}
	return `"key" IN (` + strings.Join(quoted, ",") + `)`
}

func identifySystemObject(object systemObject, references map[string]SystemReference, byName map[string][]string) string {
	for _, candidate := range []string{object.Key, object.ObjectKey, object.Label} {
		candidate = strings.ToUpper(strings.TrimSpace(candidate))
		if _, ok := references[candidate]; ok {
			return candidate
		}
		matches := systemKeyPattern.FindStringSubmatch(candidate)
		if len(matches) == 3 {
			key := strings.ToUpper(matches[2])
			if _, ok := references[key]; ok {
				return key
			}
		}
	}
	matchingKeys := byName[strings.ToLower(strings.TrimSpace(object.Label))]
	if len(matchingKeys) == 1 {
		return matchingKeys[0]
	}
	return ""
}

func objectAttributeString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return strings.TrimSpace(value), nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err == nil {
		if len(values) != 1 {
			return "", fmt.Errorf("expected one value, got %d", len(values))
		}
		return strings.TrimSpace(values[0]), nil
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err == nil {
		for _, key := range []string{"label", "name", "displayValue", "value", "key"} {
			if candidate, ok := object[key].(string); ok && strings.TrimSpace(candidate) != "" {
				return strings.TrimSpace(candidate), nil
			}
		}
	}
	return "", fmt.Errorf("unsupported attribute value %s", string(raw))
}
