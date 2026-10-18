package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/nightshift-habits/terraform-provider-qualtive/internal/client"
)

const (
	defaultTheme          = `{"background":{"type":"predefined","value":"default"},"cornerStyle":"rounded","font":{"type":"predefined","value":"default"},"isBackgroundAttachmentVisibleInUserResponses":false,"isBackgroundColorVisibleInUserResponses":false}`
	defaultSubmittedPages = `[]`

	builtInSubmittedPages = `[{"conditions":[],"content":[{"id":0,"text":{"en-US":"Thank you, sent!","sv-SE":"Tack, skickat!"},"type":"confirmationText"},{"id":0,"type":"name"},{"id":0,"type":"userInput"}],"id":0}]`
)

var slugRegexp = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

var (
	_ resource.Resource                = &enquiryResource{}
	_ resource.ResourceWithConfigure   = &enquiryResource{}
	_ resource.ResourceWithImportState = &enquiryResource{}
)

type enquiryResource struct {
	client *client.Client
}

type enquiryModel struct {
	ID                           types.String         `tfsdk:"id"`
	Container                    types.String         `tfsdk:"container"`
	Slug                         types.String         `tfsdk:"slug"`
	WorkspaceID                  types.Int64          `tfsdk:"workspace_id"`
	Name                         types.Map            `tfsdk:"name"`
	CategoryID                   types.Int64          `tfsdk:"category_id"`
	Pages                        jsontypes.Normalized `tfsdk:"pages"`
	SubmittedPages               jsontypes.Normalized `tfsdk:"submitted_pages"`
	Theme                        jsontypes.Normalized `tfsdk:"theme"`
	IsUserContactDetailsRequired types.Bool           `tfsdk:"is_user_contact_details_required"`
	HasDraft                     types.Bool           `tfsdk:"has_draft"`
}

func NewEnquiryResource() resource.Resource {
	return &enquiryResource{}
}

func (r *enquiryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_enquiry"
}

func (r *enquiryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Qualtive enquiry: the form feedback entries are collected through, addressed by its container and slug.\n\n" +
			"Creating fails if the slug is already taken in the container; import the existing enquiry instead. " +
			"Deleting is permanent: the enquiry is removed, but its feedback entries are kept.\n\n" +
			"~> The server's slug check isn't atomic. Apply with `-parallelism=1` so that two creates with the same slug can't both succeed.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Server-assigned numeric ID of the enquiry.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"container": schema.StringAttribute{
				MarkdownDescription: "ID of the container (the company account) the enquiry belongs to. Changing it creates a new enquiry.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"slug": schema.StringAttribute{
				MarkdownDescription: "URL slug of the enquiry, unique within the container. Lowercase letters, digits and `-`. Changing it renames the enquiry in place; the old slug keeps resolving for feedback posters.",
				Required:            true,
				Validators:          []validator.String{slugValidator()},
			},
			"workspace_id": schema.Int64Attribute{
				MarkdownDescription: "ID of the workspace the enquiry lives in. Changing it moves the enquiry.",
				Required:            true,
			},
			"name": schema.MapAttribute{
				MarkdownDescription: "Name of the enquiry by language code, for example `{ en = \"Contact us\" }`. Use `*` for the fallback language.",
				ElementType:         types.StringType,
				Required:            true,
				Validators:          []validator.Map{mapvalidator.SizeAtLeast(1)},
			},
			"category_id": schema.Int64Attribute{
				MarkdownDescription: "ID of the category the enquiry is filed under.",
				Optional:            true,
			},
			"pages": schema.StringAttribute{
				MarkdownDescription: "JSON array of pages shown to the feedback poster, in the Admin API format. Use `jsonencode()`. " +
					"Each page and content item needs an `id` that is unique within its page and stays stable between applies.",
				CustomType: jsontypes.NormalizedType{},
				Required:   true,
			},
			"submitted_pages": schema.StringAttribute{
				MarkdownDescription: "JSON array of pages shown after the feedback is submitted, in the Admin API format. Defaults to `[]`, which shows Qualtive's built-in thank-you page.",
				CustomType:          jsontypes.NormalizedType{},
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(defaultSubmittedPages),
			},
			"theme": schema.StringAttribute{
				MarkdownDescription: "JSON theme object in the Admin API format. Defaults to the predefined default background and font with rounded corners.",
				CustomType:          jsontypes.NormalizedType{},
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(defaultTheme),
			},
			"is_user_contact_details_required": schema.BoolAttribute{
				MarkdownDescription: "Whether feedback posters must leave contact details. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"has_draft": schema.BoolAttribute{
				MarkdownDescription: "Whether someone has unpublished edits to the enquiry in the admin UI. The provider never touches drafts.",
				Computed:            true,
			},
		},
	}
}

func (r *enquiryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *client.Client, got %T.", req.ProviderData))
		return
	}
	r.client = c
}

func (r *enquiryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan enquiryModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input, diags := plan.toInput(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	enquiry, err := r.client.CreateEnquiry(ctx, plan.Container.ValueString(), plan.WorkspaceID.ValueInt64(), input)
	if errors.Is(err, client.ErrSlugAlreadyExists) {
		resp.Diagnostics.AddAttributeError(path.Root("slug"), "Slug already exists",
			fmt.Sprintf("An enquiry with the slug %q already exists in the container %q. Import it instead:\n\n"+
				"  tofu import <resource address> %s/%s",
				plan.Slug.ValueString(), plan.Container.ValueString(), plan.Container.ValueString(), plan.Slug.ValueString()))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to create enquiry", err.Error())
		return
	}

	resp.Diagnostics.Append(plan.applyWrite(ctx, enquiry)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *enquiryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state enquiryModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	enquiry, err := r.fetch(ctx, &state)
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read enquiry", err.Error())
		return
	}

	resp.Diagnostics.Append(state.applyRead(ctx, enquiry)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *enquiryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state enquiryModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, diags := state.enquiryID()
	resp.Diagnostics.Append(diags...)
	input, diags := plan.toInput(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	enquiry, err := r.client.UpdateEnquiry(ctx, plan.Container.ValueString(), id, plan.WorkspaceID.ValueInt64(), input)
	if errors.Is(err, client.ErrSlugAlreadyExists) {
		resp.Diagnostics.AddAttributeError(path.Root("slug"), "Slug already exists",
			fmt.Sprintf("Another enquiry in the container %q already uses the slug %q.", plan.Container.ValueString(), plan.Slug.ValueString()))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to update enquiry", err.Error())
		return
	}

	plan.ID = state.ID
	resp.Diagnostics.Append(plan.applyWrite(ctx, enquiry)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *enquiryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state enquiryModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, diags := state.enquiryID()
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteEnquiry(ctx, state.Container.ValueString(), id)
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Failed to delete enquiry", err.Error())
	}
}

func (r *enquiryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	container, slug, ok := strings.Cut(req.ID, "/")
	if !ok || container == "" || !slugRegexp.MatchString(slug) {
		resp.Diagnostics.AddError("Invalid import ID",
			fmt.Sprintf("Expected an import ID of the form <container>/<slug>, got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("container"), container)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("slug"), slug)...)
}

// fetch reads by ID once it is known, so a rename made outside OpenTofu shows up as slug drift.
func (r *enquiryResource) fetch(ctx context.Context, state *enquiryModel) (*client.Enquiry, error) {
	container := state.Container.ValueString()
	if state.ID.IsNull() || state.ID.IsUnknown() {
		return r.client.GetEnquiryBySlug(ctx, container, state.WorkspaceID.ValueInt64Pointer(), state.Slug.ValueString())
	}
	id, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid enquiry ID %q in state: %w", state.ID.ValueString(), err)
	}
	return r.client.GetEnquiry(ctx, container, id)
}

func (m *enquiryModel) enquiryID() (int64, diag.Diagnostics) {
	var diags diag.Diagnostics
	id, err := strconv.ParseInt(m.ID.ValueString(), 10, 64)
	if err != nil {
		diags.AddError("Invalid enquiry ID in state", fmt.Sprintf("%q is not a numeric enquiry ID.", m.ID.ValueString()))
	}
	return id, diags
}

func (m *enquiryModel) toInput(ctx context.Context) (client.EnquiryInput, diag.Diagnostics) {
	var diags diag.Diagnostics
	input := client.EnquiryInput{
		Slug:                         m.Slug.ValueString(),
		CategoryID:                   m.CategoryID.ValueInt64Pointer(),
		Pages:                        json.RawMessage(m.Pages.ValueString()),
		SubmittedPages:               json.RawMessage(m.SubmittedPages.ValueString()),
		Theme:                        json.RawMessage(m.Theme.ValueString()),
		IsUserContactDetailsRequired: m.IsUserContactDetailsRequired.ValueBool(),
	}
	diags.Append(m.Name.ElementsAs(ctx, &input.Name, false)...)
	return input, diags
}

func (m *enquiryModel) applyRead(ctx context.Context, enquiry *client.Enquiry) diag.Diagnostics {
	diags := m.applyScalars(ctx, enquiry)
	var jsonDiags diag.Diagnostics
	m.Pages = managedJSON(enquiry.Pages, "pages", &jsonDiags)
	m.SubmittedPages = managedJSON(withoutDefaultSubmittedPage(enquiry.SubmittedPages), "submitted_pages", &jsonDiags)
	m.Theme = managedJSON(enquiry.Theme, "theme", &jsonDiags)
	diags.Append(jsonDiags...)
	return diags
}

// applyWrite keeps planned JSON values the server normalized differently, since
// returning them would fail the apply; the next plan then shows the difference.
func (m *enquiryModel) applyWrite(ctx context.Context, enquiry *client.Enquiry) diag.Diagnostics {
	diags := m.applyScalars(ctx, enquiry)
	m.Pages = plannedOrServerJSON(ctx, m.Pages, enquiry.Pages, "pages", &diags)
	m.SubmittedPages = plannedOrServerJSON(ctx, m.SubmittedPages, withoutDefaultSubmittedPage(enquiry.SubmittedPages), "submitted_pages", &diags)
	m.Theme = plannedOrServerJSON(ctx, m.Theme, enquiry.Theme, "theme", &diags)
	return diags
}

// withoutDefaultSubmittedPage maps the built-in thank-you page back to `[]`: the backend
// stores `[]` for it and returns the built-in page in its place.
func withoutDefaultSubmittedPage(raw json.RawMessage) json.RawMessage {
	stripped, err := stripComputedJSON(raw)
	if err == nil && stripped == builtInSubmittedPages {
		return json.RawMessage(defaultSubmittedPages)
	}
	return raw
}

func (m *enquiryModel) applyScalars(ctx context.Context, enquiry *client.Enquiry) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(strconv.FormatInt(enquiry.ID, 10))
	m.Slug = types.StringValue(enquiry.Slug)
	m.WorkspaceID = types.Int64Value(enquiry.WorkspaceID)
	m.CategoryID = types.Int64PointerValue(enquiry.CategoryID)
	m.IsUserContactDetailsRequired = types.BoolValue(enquiry.IsUserContactDetailsRequired)
	m.HasDraft = types.BoolValue(enquiry.HasDraft())
	name, nameDiags := types.MapValueFrom(ctx, types.StringType, enquiry.Name)
	diags.Append(nameDiags...)
	m.Name = name
	return diags
}

func plannedOrServerJSON(ctx context.Context, planned jsontypes.Normalized, raw json.RawMessage, attribute string, diags *diag.Diagnostics) jsontypes.Normalized {
	server := managedJSON(raw, attribute, diags)
	if planned.IsUnknown() || planned.IsNull() {
		return server
	}
	equal, equalDiags := planned.StringSemanticEquals(ctx, server)
	diags.Append(equalDiags...)
	if equal {
		return planned
	}
	diags.AddAttributeWarning(path.Root(attribute), "Qualtive normalized "+attribute,
		"The Admin API stored a different value than the one configured, so the next plan will show a difference. "+
			"Update the configuration to match the stored value:\n\n"+server.ValueString())
	return planned
}

// managedJSON drops the fields Qualtive computes after a write, such as machine translations.
func managedJSON(raw json.RawMessage, attribute string, diags *diag.Diagnostics) jsontypes.Normalized {
	cleaned, err := stripComputedJSON(raw)
	if err != nil {
		diags.AddError("Unexpected "+attribute+" from the Admin API", err.Error())
		return jsontypes.NewNormalizedNull()
	}
	return jsontypes.NewNormalizedValue(cleaned)
}

func stripComputedJSON(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "null", nil
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	data, err := json.Marshal(stripComputedFields(value))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func stripComputedFields(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.HasPrefix(key, "automaticTranslated") {
				delete(typed, key)
				continue
			}
			typed[key] = stripComputedFields(child)
		}
	case []any:
		for i, child := range typed {
			typed[i] = stripComputedFields(child)
		}
	}
	return value
}

func slugValidator() validator.String {
	return stringvalidator.RegexMatches(slugRegexp, "must start with a lowercase letter or digit and contain only lowercase letters, digits and -")
}
