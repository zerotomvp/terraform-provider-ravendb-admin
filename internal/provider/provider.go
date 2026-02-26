package provider

import (
	"context"

	"github.com/zerotomvp/terraform-provider-ravendb-admin/internal/client"
	"github.com/zerotomvp/terraform-provider-ravendb-admin/internal/datasources"
	"github.com/zerotomvp/terraform-provider-ravendb-admin/internal/resources"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure RavenDBProvider satisfies various provider interfaces.
var _ provider.Provider = &RavenDBProvider{}

// RavenDBProvider defines the provider implementation.
type RavenDBProvider struct {
	version string
}

// RavenDBProviderModel describes the provider data model.
type RavenDBProviderModel struct {
	URL                 types.String `tfsdk:"url"`
	CertificatePath     types.String `tfsdk:"certificate_path"`
	CertificatePassword types.String `tfsdk:"certificate_password"`
	InsecureSkipVerify  types.Bool   `tfsdk:"insecure_skip_verify"`
}

func (p *RavenDBProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "ravendb"
	resp.Version = p.version
}

func (p *RavenDBProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Terraform provider for managing RavenDB databases and certificates.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Description: "The URL of the RavenDB server (e.g., http://localhost:8080 or https://localhost:8443).",
				Required:    true,
			},
			"certificate_path": schema.StringAttribute{
				Description: "Path to the client certificate file (PEM or PFX format) for authenticated connections.",
				Optional:    true,
			},
			"certificate_password": schema.StringAttribute{
				Description: "Password for the client certificate (if protected).",
				Optional:    true,
				Sensitive:   true,
			},
			"insecure_skip_verify": schema.BoolAttribute{
				Description: "Skip TLS certificate verification. Use only for testing with self-signed certificates.",
				Optional:    true,
			},
		},
	}
}

func (p *RavenDBProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data RavenDBProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Create the client configuration
	config := client.ClientConfig{
		URL:                data.URL.ValueString(),
		InsecureSkipVerify: data.InsecureSkipVerify.ValueBool(),
	}

	if !data.CertificatePath.IsNull() {
		config.CertificatePath = data.CertificatePath.ValueString()
	}

	if !data.CertificatePassword.IsNull() {
		config.CertificatePassword = data.CertificatePassword.ValueString()
	}

	// Create the RavenDB client
	ravenClient, err := client.NewClient(config)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create RavenDB Client",
			"An unexpected error occurred when creating the RavenDB client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"Error: "+err.Error(),
		)
		return
	}

	// Verify connectivity
	if err := ravenClient.Ping(); err != nil {
		resp.Diagnostics.AddError(
			"Unable to Connect to RavenDB",
			"The provider could not connect to the RavenDB server. "+
				"Please verify the URL and credentials are correct.\n\n"+
				"Error: "+err.Error(),
		)
		return
	}

	// Make the client available to resources and data sources
	resp.DataSourceData = ravenClient
	resp.ResourceData = ravenClient
}

func (p *RavenDBProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewDatabaseResource,
		resources.NewCertificateResource,
	}
}

func (p *RavenDBProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		datasources.NewDatabasesDataSource,
		datasources.NewCertificateDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &RavenDBProvider{
			version: version,
		}
	}
}
