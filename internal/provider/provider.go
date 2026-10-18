package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/nightshift-habits/terraform-provider-qualtive/internal/client"
)

const (
	DefaultEndpoint = "https://admin-api.qualtive.io"
	EnvAPIKey       = "QUALTIVE_API_KEY"
	EnvEndpoint     = "QUALTIVE_ENDPOINT"
)

var _ provider.Provider = &qualtiveProvider{}

type qualtiveProvider struct {
	version string
}

type qualtiveProviderModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	APIKey   types.String `tfsdk:"api_key"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &qualtiveProvider{version: version}
	}
}

func (p *qualtiveProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "qualtive"
	resp.Version = p.version
}

func (p *qualtiveProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage [Qualtive](https://qualtive.io) resources through the Qualtive Admin API.\n\n" +
			"~> **Alpha:** this provider is in early development. Resources, attributes and behaviour may change in any `0.x` release.\n\n" +
			"-> Until the Admin API fix for API-key roles is deployed, a key acts one role below its user's role, so writes need the key of a user with the **admin** role.\n\n" +
			"Creating enquiries isn't atomic on the server, so two creates with the same slug can both succeed. Apply with `-parallelism=1`, and don't let two configurations manage the same container at the same time.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "Base URL of the Qualtive Admin API. Defaults to `" + DefaultEndpoint + "`, or the `" + EnvEndpoint + "` environment variable when set.",
				Optional:            true,
			},
			"api_key": schema.StringAttribute{
				MarkdownDescription: "User API key (`u-…`) with the editor or admin role in every container you manage. Can also be set with the `" + EnvAPIKey + "` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
		},
	}
}

func (p *qualtiveProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config qualtiveProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.Endpoint.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"), "Unknown Qualtive endpoint",
			"The endpoint must be known when the provider is configured.")
	}
	if config.APIKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("api_key"), "Unknown Qualtive API key",
			"The API key must be known when the provider is configured.")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := firstNonEmpty(config.Endpoint.ValueString(), os.Getenv(EnvEndpoint), DefaultEndpoint)
	apiKey := firstNonEmpty(config.APIKey.ValueString(), os.Getenv(EnvAPIKey))
	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(path.Root("api_key"), "Missing Qualtive API key",
			"Set the api_key provider attribute or the "+EnvAPIKey+" environment variable.")
		return
	}

	c := client.New(endpoint, apiKey, "terraform-provider-qualtive/"+p.version)
	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *qualtiveProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewEnquiryResource,
	}
}

func (p *qualtiveProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewEnquiryDataSource,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
