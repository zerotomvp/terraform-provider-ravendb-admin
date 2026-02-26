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
var _ datasource.DataSource = &DatabasesDataSource{}

func NewDatabasesDataSource() datasource.DataSource {
	return &DatabasesDataSource{}
}

// DatabasesDataSource defines the data source implementation.
type DatabasesDataSource struct {
	client *client.Client
}

// DatabasesDataSourceModel describes the data source data model.
type DatabasesDataSourceModel struct {
	Databases []DatabaseModel `tfsdk:"databases"`
}

// DatabaseModel describes a database in the list.
type DatabaseModel struct {
	Name           types.String `tfsdk:"name"`
	Disabled       types.Bool   `tfsdk:"disabled"`
	Encrypted      types.Bool   `tfsdk:"encrypted"`
	DocumentsCount types.Int64  `tfsdk:"documents_count"`
	IndexesCount   types.Int64  `tfsdk:"indexes_count"`
}

func (d *DatabasesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_databases"
}

func (d *DatabasesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists all databases in the RavenDB cluster.",

		Attributes: map[string]schema.Attribute{
			"databases": schema.ListNestedAttribute{
				Description: "List of databases.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The name of the database.",
							Computed:    true,
						},
						"disabled": schema.BoolAttribute{
							Description: "Whether the database is disabled.",
							Computed:    true,
						},
						"encrypted": schema.BoolAttribute{
							Description: "Whether the database is encrypted.",
							Computed:    true,
						},
						"documents_count": schema.Int64Attribute{
							Description: "Number of documents in the database.",
							Computed:    true,
						},
						"indexes_count": schema.Int64Attribute{
							Description: "Number of indexes in the database.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *DatabasesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *DatabasesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data DatabasesDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	databases, err := d.client.ListDatabases()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Databases",
			"Could not list databases: "+err.Error(),
		)
		return
	}

	data.Databases = make([]DatabaseModel, len(databases))
	for i, db := range databases {
		data.Databases[i] = DatabaseModel{
			Name:           types.StringValue(db.Name),
			Disabled:       types.BoolValue(db.Disabled),
			Encrypted:      types.BoolValue(db.IsEncrypted),
			DocumentsCount: types.Int64Value(db.DocumentsCount),
			IndexesCount:   types.Int64Value(int64(db.IndexesCount)),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
