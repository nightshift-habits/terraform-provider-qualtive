package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const fakeAPIKey = "u-fake"

// fakeAdminAPI is an in-memory stand-in for the enquiry endpoints of the Admin API,
// following the contract the real backend implements, including machine-translated fields.
type fakeAdminAPI struct {
	server *httptest.Server

	mu        sync.Mutex
	nextID    int64
	enquiries map[string]map[int64]*fakeEnquiry
}

type fakeEnquiry struct {
	ID                           int64             `json:"id"`
	Slug                         string            `json:"slug"`
	Name                         map[string]string `json:"name"`
	WorkspaceID                  int64             `json:"workspaceId"`
	CategoryID                   *int64            `json:"categoryId"`
	Pages                        json.RawMessage   `json:"pages"`
	SubmittedPages               json.RawMessage   `json:"submittedPages"`
	Theme                        json.RawMessage   `json:"theme"`
	IsUserContactDetailsRequired bool              `json:"isUserContactDetailsRequired"`
}

func newFakeAdminAPI(t *testing.T) *fakeAdminAPI {
	t.Helper()
	api := &fakeAdminAPI{nextID: 4000, enquiries: map[string]map[int64]*fakeEnquiry{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /feedback/questions/by-slug/{slug}", api.getBySlug)
	mux.HandleFunc("GET /feedback/questions/{id}", api.get)
	mux.HandleFunc("POST /feedback/questions", api.create)
	mux.HandleFunc("PUT /feedback/questions/{id}", api.update)
	mux.HandleFunc("DELETE /feedback/questions/{id}", api.delete)
	api.server = httptest.NewServer(api.authorize(mux))
	t.Cleanup(api.server.Close)
	return api
}

func (api *fakeAdminAPI) mutate(container, slug string, change func(*fakeEnquiry)) {
	api.mu.Lock()
	defer api.mu.Unlock()
	if enquiry := api.findBySlug(container, nil, slug); enquiry != nil {
		change(enquiry)
	}
}

func (api *fakeAdminAPI) remove(container, slug string) {
	api.mu.Lock()
	defer api.mu.Unlock()
	if enquiry := api.findBySlug(container, nil, slug); enquiry != nil {
		delete(api.enquiries[container], enquiry.ID)
	}
}

func (api *fakeAdminAPI) authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeAPIKey {
			writeReason(w, http.StatusUnauthorized, "InvalidAPIKey")
			return
		}
		if r.Header.Get("X-Container") == "" {
			writeReason(w, http.StatusBadRequest, "Missing X-Container")
			return
		}
		api.mu.Lock()
		defer api.mu.Unlock()
		next.ServeHTTP(w, r)
	})
}

func (api *fakeAdminAPI) getBySlug(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := optionalWorkspace(w, r)
	if !ok {
		return
	}
	enquiry := api.findBySlug(r.Header.Get("X-Container"), workspaceID, normalizeSlug(r.PathValue("slug")))
	if enquiry == nil {
		writeReason(w, http.StatusNotFound, "NotFound")
		return
	}
	writeEnquiry(w, enquiry)
}

func (api *fakeAdminAPI) get(w http.ResponseWriter, r *http.Request) {
	enquiry := api.findByID(w, r)
	if enquiry == nil {
		return
	}
	writeEnquiry(w, enquiry)
}

func (api *fakeAdminAPI) create(w http.ResponseWriter, r *http.Request) {
	container := r.Header.Get("X-Container")
	workspaceID, err := strconv.ParseInt(r.Header.Get("X-Workspace"), 10, 64)
	if err != nil {
		writeReason(w, http.StatusBadRequest, "Missing X-Workspace")
		return
	}
	var body struct {
		fakeEnquiry
		OnSlugConflict string `json:"onSlugConflict"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Name) == 0 || len(body.Pages) == 0 ||
		len(body.SubmittedPages) == 0 || len(body.Theme) == 0 {
		writeReason(w, http.StatusBadRequest, "InvalidBody")
		return
	}
	if body.OnSlugConflict != "fail" || body.Slug == "" {
		writeReason(w, http.StatusBadRequest, "provider must send onSlugConflict=fail and a slug")
		return
	}
	enquiry := body.fakeEnquiry
	enquiry.Slug = normalizeSlug(enquiry.Slug)
	if api.findBySlug(container, nil, enquiry.Slug) != nil {
		writeReason(w, http.StatusConflict, "SlugAlreadyExists")
		return
	}
	api.nextID++
	enquiry.ID = api.nextID
	enquiry.WorkspaceID = workspaceID
	if api.enquiries[container] == nil {
		api.enquiries[container] = map[int64]*fakeEnquiry{}
	}
	api.enquiries[container][enquiry.ID] = &enquiry
	writeEnquiry(w, &enquiry)
}

func (api *fakeAdminAPI) update(w http.ResponseWriter, r *http.Request) {
	enquiry := api.findByID(w, r)
	if enquiry == nil {
		return
	}
	var fields map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
		writeReason(w, http.StatusBadRequest, "InvalidBody")
		return
	}
	if string(fields["onSlugConflict"]) != `"fail"` {
		writeReason(w, http.StatusBadRequest, "provider must send onSlugConflict=fail")
		return
	}
	updated := *enquiry
	if _, ok := fields["name"]; ok {
		updated.Name = nil
	}
	if err := json.Unmarshal(mustMarshal(fields), &updated); err != nil {
		writeReason(w, http.StatusBadRequest, "InvalidBody")
		return
	}
	updated.Slug = normalizeSlug(updated.Slug)
	if other := api.findBySlug(r.Header.Get("X-Container"), nil, updated.Slug); other != nil && other.ID != enquiry.ID {
		writeReason(w, http.StatusConflict, "SlugAlreadyExists")
		return
	}
	*enquiry = updated
	writeEnquiry(w, enquiry)
}

func (api *fakeAdminAPI) delete(w http.ResponseWriter, r *http.Request) {
	enquiry := api.findByID(w, r)
	if enquiry == nil {
		return
	}
	delete(api.enquiries[r.Header.Get("X-Container")], enquiry.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (api *fakeAdminAPI) findByID(w http.ResponseWriter, r *http.Request) *fakeEnquiry {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeReason(w, http.StatusBadRequest, "InvalidID")
		return nil
	}
	enquiry := api.enquiries[r.Header.Get("X-Container")][id]
	if enquiry == nil {
		writeReason(w, http.StatusNotFound, "NotFound")
	}
	return enquiry
}

func (api *fakeAdminAPI) findBySlug(container string, workspaceID *int64, slug string) *fakeEnquiry {
	for _, enquiry := range api.enquiries[container] {
		if enquiry.Slug == slug && (workspaceID == nil || *workspaceID == enquiry.WorkspaceID) {
			return enquiry
		}
	}
	return nil
}

func optionalWorkspace(w http.ResponseWriter, r *http.Request) (*int64, bool) {
	header := r.Header.Get("X-Workspace")
	if header == "" {
		return nil, true
	}
	id, err := strconv.ParseInt(header, 10, 64)
	if err != nil {
		writeReason(w, http.StatusBadRequest, "Invalid X-Workspace")
		return nil, false
	}
	return &id, true
}

func normalizeSlug(slug string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(slug)), " ", "-")
}

// writeEnquiry adds the computed fields and the built-in thank-you page the real API returns,
// so tests prove the provider handles them.
func writeEnquiry(w http.ResponseWriter, enquiry *fakeEnquiry) {
	var response map[string]any
	_ = json.Unmarshal(mustMarshal(enquiry), &response)
	response["automaticTranslatedName"] = map[string]string{"xx": "machine translated"}
	response["pages"] = withAutomaticTranslations(response["pages"])
	if pages, _ := response["submittedPages"].([]any); len(pages) == 0 {
		var builtIn any
		_ = json.Unmarshal([]byte(builtInSubmittedPages), &builtIn)
		response["submittedPages"] = withAutomaticTranslations(builtIn)
	}
	response["workspace"] = map[string]any{"id": enquiry.WorkspaceID}
	response["draft"] = nil
	response["viewCount"] = 42
	response["updated"] = 1760000000000
	response["created"] = 1700000000000
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func withAutomaticTranslations(pages any) any {
	pageList, _ := pages.([]any)
	for _, page := range pageList {
		pageObject, _ := page.(map[string]any)
		content, _ := pageObject["content"].([]any)
		for _, item := range content {
			if itemObject, ok := item.(map[string]any); ok && itemObject["text"] != nil {
				itemObject["automaticTranslatedText"] = map[string]string{"xx": "machine translated"}
			}
		}
	}
	return pages
}

func writeReason(w http.ResponseWriter, status int, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"reason": reason})
}

func mustMarshal(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}
