package datasources

import (
	"context"
	"fmt"

	"github.com/zerotomvp/terraform-provider-ravendb-admin/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &CertificateDataSource{}

func NewCertificateDataSource() datasource.DataSource {
	return &CertificateDataSource{}
}

// CertificateDataSource defines the data source implementation.
type CertificateDataSource struct {
	client *client.Client
}

// CertificateDataSourceModel describes the data source data model.
type CertificateDataSourceModel struct {
	Thumbprint        types.String `tfsdk:"thumbprint"`
	Name              types.String `tfsdk:"name"`
	SecurityClearance types.String `tfsdk:"security_clearance"`
	NotAfter          types.String `tfsdk:"not_after"`
	NotBefore         types.String `tfsdk:"not_before"`
	Permissions       types.Map    `tfsdk:"permissions"`
}

func (d *CertificateDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_certificate"
}

func (d *CertificateDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves information about a RavenDB certificate by thumbprint.",

		Attributes: map[string]schema.Attribute{
			"thumbprint": schema.StringAttribute{
				Description: "The thumbprint of the certificate to look up.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the certificate.",
				Computed:    true,
			},
			"security_clearance": schema.StringAttribute{
				Description: "Security clearance level of the certificate.",
				Computed:    true,
			},
			"not_after": schema.StringAttribute{
				Description: "Certificate expiration date.",
				Computed:    true,
			},
			"not_before": schema.StringAttribute{
				Description: "Certificate valid from date.",
				Computed:    true,
			},
			"permissions": schema.MapAttribute{
				Description: "Database permissions for this certificate.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *CertificateDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *CertificateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data CertificateDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cert, err := d.client.GetCertificate(data.Thumbprint.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Certificate",
			"Could not read certificate: "+err.Error(),
		)
		return
	}

	if cert == nil {
		resp.Diagnostics.AddError(
			"Certificate Not Found",
			fmt.Sprintf("No certificate found with thumbprint: %s", data.Thumbprint.ValueString()),
		)
		return
	}

	data.Name = types.StringValue(cert.Name)
	data.SecurityClearance = types.StringValue(cert.SecurityClearance)
	data.NotAfter = types.StringValue(cert.NotAfter)
	data.NotBefore = types.StringValue(cert.NotBefore)

	if cert.Permissions != nil && len(cert.Permissions) > 0 {
		permissionsMap, diags := types.MapValueFrom(ctx, types.StringType, cert.Permissions)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.Permissions = permissionsMap
	} else {
		data.Permissions = types.MapNull(types.StringType)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
