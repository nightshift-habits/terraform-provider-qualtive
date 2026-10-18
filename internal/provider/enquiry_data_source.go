package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/nightshift-habits/terraform-provider-qualtive/internal/client"
)

var (
	_ datasource.DataSource              = &enquiryDataSource{}
	_ datasource.DataSourceWithConfigure = &enquiryDataSource{}
)

type enquiryDataSource struct {
	client *client.Client
}

func NewEnquiryDataSource() datasource.DataSource {
	return &enquiryDataSource{}
}

func (d *enquiryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_enquiry"
}

func (d *enquiryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Qualtive enquiry by its current slug. Old slugs of renamed enquiries don't match.",
		Attributes: map[string]schema.Attribute{
			"container": schema.StringAttribute{
				MarkdownDescription: "ID of the container the enquiry belongs to.",
				Required:            true,
			},
			"slug": schema.StringAttribute{
				MarkdownDescription: "Current slug of the enquiry.",
				Required:            true,
				Validators:          []validator.String{slugValidator()},
			},
			"workspace_id": schema.Int64Attribute{
				MarkdownDescription: "ID of the workspace to look in. When omitted, the first enquiry with the slug in the container is used.",
				Optional:            true,
				Computed:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Numeric ID of the enquiry.",
				Computed:            true,
			},
			"name": schema.MapAttribute{
				MarkdownDescription: "Name of the enquiry by language code.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"category_id": schema.Int64Attribute{
				MarkdownDescription: "ID of the category the enquiry is filed under, if any.",
				Computed:            true,
			},
			"pages": schema.StringAttribute{
				MarkdownDescription: "JSON array of pages, without machine-translated fields.",
				CustomType:          jsontypes.NormalizedType{},
				Computed:            true,
			},
			"submitted_pages": schema.StringAttribute{
				MarkdownDescription: "JSON array of pages shown after submitting, without machine-translated fields.",
				CustomType:          jsontypes.NormalizedType{},
				Computed:            true,
			},
			"theme": schema.StringAttribute{
				MarkdownDescription: "JSON theme object.",
				CustomType:          jsontypes.NormalizedType{},
				Computed:            true,
			},
			"is_user_contact_details_required": schema.BoolAttribute{
				MarkdownDescription: "Whether feedback posters must leave contact details.",
				Computed:            true,
			},
			"has_draft": schema.BoolAttribute{
				MarkdownDescription: "Whether someone has unpublished edits to the enquiry in the admin UI.",
				Computed:            true,
			},
		},
	}
}

func (d *enquiryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *client.Client, got %T.", req.ProviderData))
		return
	}
	d.client = c
}

func (d *enquiryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config enquiryModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	enquiry, err := d.client.GetEnquiryBySlug(ctx, config.Container.ValueString(), config.WorkspaceID.ValueInt64Pointer(), config.Slug.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Enquiry not found",
			fmt.Sprintf("No enquiry in the container %q has the slug %q.", config.Container.ValueString(), config.Slug.ValueString()))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read enquiry", err.Error())
		return
	}

	resp.Diagnostics.Append(config.applyRead(ctx, enquiry)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
