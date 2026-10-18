package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type recordedRequest struct {
	method  string
	path    string
	headers http.Header
	body    map[string]any
}

func newTestServer(t *testing.T, status int, response string) (*Client, *recordedRequest) {
	t.Helper()
	recorded := &recordedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded.method = r.Method
		recorded.path = r.URL.EscapedPath()
		recorded.headers = r.Header.Clone()
		data, _ := io.ReadAll(r.Body)
		if len(data) > 0 {
			if err := json.Unmarshal(data, &recorded.body); err != nil {
				t.Errorf("request body is not JSON: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)
	return New(server.URL+"/", "u-key", "test-agent"), recorded
}

const enquiryJSON = `{"id":4404,"slug":"contact","name":{"en":"Contact us"},"workspaceId":17,"categoryId":null,
"pages":[],"submittedPages":[],"theme":{},"isUserContactDetailsRequired":true,"draft":null,"viewCount":3}`

func TestGetEnquiryBySlug(t *testing.T) {
	c, recorded := newTestServer(t, http.StatusOK, enquiryJSON)
	workspaceID := int64(17)

	enquiry, err := c.GetEnquiryBySlug(context.Background(), "qualtive", &workspaceID, "rate this")
	if err != nil {
		t.Fatal(err)
	}

	assertEqual(t, recorded.method, http.MethodGet)
	assertEqual(t, recorded.path, "/feedback/questions/by-slug/rate%20this")
	assertEqual(t, recorded.headers.Get("Authorization"), "Bearer u-key")
	assertEqual(t, recorded.headers.Get("X-Container"), "qualtive")
	assertEqual(t, recorded.headers.Get("X-Workspace"), "17")
	assertEqual(t, recorded.headers.Get("API-Version"), APIVersion)
	assertEqual(t, recorded.headers.Get("User-Agent"), "test-agent")
	assertEqual(t, enquiry.ID, int64(4404))
	assertEqual(t, enquiry.Slug, "contact")
	assertEqual(t, enquiry.WorkspaceID, int64(17))
	assertEqual(t, enquiry.CategoryID, (*int64)(nil))
	assertEqual(t, enquiry.IsUserContactDetailsRequired, true)
	assertEqual(t, enquiry.HasDraft(), false)
}

func TestGetEnquiryBySlugWithoutWorkspace(t *testing.T) {
	c, recorded := newTestServer(t, http.StatusOK, enquiryJSON)

	if _, err := c.GetEnquiryBySlug(context.Background(), "qualtive", nil, "contact"); err != nil {
		t.Fatal(err)
	}

	if _, ok := recorded.headers["X-Workspace"]; ok {
		t.Errorf("X-Workspace should not be sent, got %q", recorded.headers.Get("X-Workspace"))
	}
}

func TestCreateEnquiryAlwaysFailsOnSlugConflict(t *testing.T) {
	c, recorded := newTestServer(t, http.StatusOK, enquiryJSON)
	categoryID := int64(3)

	_, err := c.CreateEnquiry(context.Background(), "qualtive", 17, EnquiryInput{
		Slug:           "contact",
		OnSlugConflict: "suffix",
		Name:           LocalizedString{"en": "Contact us"},
		CategoryID:     &categoryID,
		Pages:          json.RawMessage(`[{"id":1,"content":[]}]`),
		SubmittedPages: json.RawMessage(`[]`),
		Theme:          json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	assertEqual(t, recorded.method, http.MethodPost)
	assertEqual(t, recorded.path, "/feedback/questions")
	assertEqual(t, recorded.headers.Get("X-Workspace"), "17")
	assertEqual(t, recorded.headers.Get("Content-Type"), "application/json")
	assertEqual(t, recorded.body["onSlugConflict"], "fail")
	assertEqual(t, recorded.body["slug"], "contact")
	assertEqual[any](t, recorded.body["categoryId"], float64(3))
	assertEqual(t, recorded.body["isUserContactDetailsRequired"], false)
}

func TestUpdateEnquirySendsExplicitNullCategoryAndWorkspace(t *testing.T) {
	c, recorded := newTestServer(t, http.StatusOK, enquiryJSON)

	_, err := c.UpdateEnquiry(context.Background(), "qualtive", 4404, 18, EnquiryInput{
		Slug:           "contact",
		Name:           LocalizedString{"en": "Contact us"},
		Pages:          json.RawMessage(`[]`),
		SubmittedPages: json.RawMessage(`[]`),
		Theme:          json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	assertEqual(t, recorded.method, http.MethodPut)
	assertEqual(t, recorded.path, "/feedback/questions/4404")
	assertEqual(t, recorded.body["onSlugConflict"], "fail")
	assertEqual[any](t, recorded.body["workspaceId"], float64(18))
	value, ok := recorded.body["categoryId"]
	if !ok || value != nil {
		t.Errorf("categoryId should be an explicit null, got %v (present: %v)", value, ok)
	}
}

func TestDeleteEnquiry(t *testing.T) {
	c, recorded := newTestServer(t, http.StatusNoContent, "")

	if err := c.DeleteEnquiry(context.Background(), "qualtive", 4404); err != nil {
		t.Fatal(err)
	}

	assertEqual(t, recorded.method, http.MethodDelete)
	assertEqual(t, recorded.path, "/feedback/questions/4404")
}

func TestErrors(t *testing.T) {
	tests := []struct {
		status  int
		body    string
		target  error
		message string
	}{
		{http.StatusNotFound, `{"reason":"NotFound"}`, ErrNotFound, "qualtive admin API returned HTTP 404: NotFound"},
		{http.StatusConflict, `{"reason":"SlugAlreadyExists"}`, ErrSlugAlreadyExists, "qualtive admin API returned HTTP 409: SlugAlreadyExists"},
		{http.StatusForbidden, `not json`, nil, "qualtive admin API returned HTTP 403"},
	}
	for _, test := range tests {
		c, _ := newTestServer(t, test.status, test.body)

		_, err := c.GetEnquiry(context.Background(), "qualtive", 1)

		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("HTTP %d: expected *APIError, got %v", test.status, err)
		}
		assertEqual(t, apiErr.StatusCode, test.status)
		assertEqual(t, err.Error(), test.message)
		if test.target != nil && !errors.Is(err, test.target) {
			t.Errorf("HTTP %d: expected errors.Is(err, %v)", test.status, test.target)
		}
		if errors.Is(err, ErrNotFound) && test.target != ErrNotFound {
			t.Errorf("HTTP %d: should not be ErrNotFound", test.status)
		}
	}
}

func TestHasDraft(t *testing.T) {
	assertEqual(t, (&Enquiry{}).HasDraft(), false)
	assertEqual(t, (&Enquiry{Draft: json.RawMessage(`null`)}).HasDraft(), false)
	assertEqual(t, (&Enquiry{Draft: json.RawMessage(`{"name":{}}`)}).HasDraft(), true)
}

func assertEqual[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}
