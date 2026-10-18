package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

const slugConflictFail = "fail"

type LocalizedString map[string]string

type Enquiry struct {
	ID                           int64           `json:"id"`
	Slug                         string          `json:"slug"`
	Name                         LocalizedString `json:"name"`
	WorkspaceID                  int64           `json:"workspaceId"`
	CategoryID                   *int64          `json:"categoryId"`
	Pages                        json.RawMessage `json:"pages"`
	SubmittedPages               json.RawMessage `json:"submittedPages"`
	Theme                        json.RawMessage `json:"theme"`
	IsUserContactDetailsRequired bool            `json:"isUserContactDetailsRequired"`
	Draft                        json.RawMessage `json:"draft"`
}

func (e *Enquiry) HasDraft() bool {
	return len(e.Draft) > 0 && string(e.Draft) != "null"
}

type EnquiryInput struct {
	Slug                         string          `json:"slug"`
	OnSlugConflict               string          `json:"onSlugConflict"`
	Name                         LocalizedString `json:"name"`
	CategoryID                   *int64          `json:"categoryId"`
	Pages                        json.RawMessage `json:"pages"`
	SubmittedPages               json.RawMessage `json:"submittedPages"`
	Theme                        json.RawMessage `json:"theme"`
	IsUserContactDetailsRequired bool            `json:"isUserContactDetailsRequired"`
}

type enquiryUpdateInput struct {
	EnquiryInput
	WorkspaceID int64 `json:"workspaceId"`
}

func (c *Client) GetEnquiry(ctx context.Context, container string, id int64) (*Enquiry, error) {
	var enquiry Enquiry
	err := c.do(ctx, http.MethodGet, enquiryPath(id), requestOptions{container: container, out: &enquiry})
	if err != nil {
		return nil, err
	}
	return &enquiry, nil
}

// GetEnquiryBySlug only matches the enquiry's current slug, not its slug history.
func (c *Client) GetEnquiryBySlug(ctx context.Context, container string, workspaceID *int64, slug string) (*Enquiry, error) {
	var enquiry Enquiry
	err := c.do(ctx, http.MethodGet, enquiryBySlugPath(slug), requestOptions{
		container:   container,
		workspaceID: workspaceID,
		out:         &enquiry,
	})
	if err != nil {
		return nil, err
	}
	return &enquiry, nil
}

// CreateEnquiry fails with ErrSlugAlreadyExists instead of picking a suffixed slug.
func (c *Client) CreateEnquiry(ctx context.Context, container string, workspaceID int64, input EnquiryInput) (*Enquiry, error) {
	input.OnSlugConflict = slugConflictFail
	var enquiry Enquiry
	err := c.do(ctx, http.MethodPost, "/feedback/questions", requestOptions{
		container:   container,
		workspaceID: &workspaceID,
		body:        input,
		out:         &enquiry,
	})
	if err != nil {
		return nil, err
	}
	return &enquiry, nil
}

// UpdateEnquiry sends every owned field, so fields not in input are reset rather than left unchanged.
func (c *Client) UpdateEnquiry(ctx context.Context, container string, id int64, workspaceID int64, input EnquiryInput) (*Enquiry, error) {
	input.OnSlugConflict = slugConflictFail
	var enquiry Enquiry
	err := c.do(ctx, http.MethodPut, enquiryPath(id), requestOptions{
		container: container,
		body:      enquiryUpdateInput{EnquiryInput: input, WorkspaceID: workspaceID},
		out:       &enquiry,
	})
	if err != nil {
		return nil, err
	}
	return &enquiry, nil
}

func (c *Client) DeleteEnquiry(ctx context.Context, container string, id int64) error {
	return c.do(ctx, http.MethodDelete, enquiryPath(id), requestOptions{container: container})
}

func enquiryPath(id int64) string {
	return "/feedback/questions/" + strconv.FormatInt(id, 10)
}

func enquiryBySlugPath(slug string) string {
	return "/feedback/questions/by-slug/" + url.PathEscape(slug)
}
