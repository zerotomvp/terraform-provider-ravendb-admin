package resources

import (
	"context"
	"fmt"

	"github.com/zerotomvp/terraform-provider-ravendb-admin/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &DatabaseResource{}
var _ resource.ResourceWithImportState = &DatabaseResource{}

func NewDatabaseResource() resource.Resource {
	return &DatabaseResource{}
}

// DatabaseResource defines the resource implementation.
type DatabaseResource struct {
	client *client.Client
}

// DatabaseResourceModel describes the resource data model.
type DatabaseResourceModel struct {
	Name              types.String `tfsdk:"name"`
	ReplicationFactor types.Int64  `tfsdk:"replication_factor"`
	Disabled          types.Bool   `tfsdk:"disabled"`
	Encrypted         types.Bool   `tfsdk:"encrypted"`
	Settings          types.Map    `tfsdk:"settings"`
}

func (r *DatabaseResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database"
}

func (r *DatabaseResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a RavenDB database.",

		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "The name of the database.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"replication_factor": schema.Int64Attribute{
				Description: "The number of nodes the database should be replicated to. Default is 1.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(1),
			},
			"disabled": schema.BoolAttribute{
				Description: "Whether the database is disabled. Default is false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"encrypted": schema.BoolAttribute{
				Description: "Whether the database is encrypted. Default is false. Cannot be changed after creation.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					// Encryption cannot be changed after creation
				},
			},
			"settings": schema.MapAttribute{
				Description: "Database settings as key-value pairs.",
				Optional:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *DatabaseResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *DatabaseResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DatabaseResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Convert settings map
	var settings map[string]string
	if !data.Settings.IsNull() {
		settings = make(map[string]string)
		resp.Diagnostics.Append(data.Settings.ElementsAs(ctx, &settings, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Create the database
	err := r.client.CreateDatabase(
		data.Name.ValueString(),
		int(data.ReplicationFactor.ValueInt64()),
		data.Disabled.ValueBool(),
		data.Encrypted.ValueBool(),
		settings,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Database",
			"Could not create database: "+err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DatabaseResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DatabaseResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get the database
	db, err := r.client.GetDatabase(data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Database",
			"Could not read database: "+err.Error(),
		)
		return
	}

	if db == nil {
		// Database was deleted outside of Terraform
		resp.State.RemoveResource(ctx)
		return
	}

	// Update state with values from API
	data.Disabled = types.BoolValue(db.Disabled)
	data.Encrypted = types.BoolValue(db.Encrypted)

	// Calculate replication factor from topology members
	if db.Topology != nil && len(db.Topology.Members) > 0 {
		data.ReplicationFactor = types.Int64Value(int64(len(db.Topology.Members)))
	}

	if db.Settings != nil && len(db.Settings) > 0 {
		settingsMap, diags := types.MapValueFrom(ctx, types.StringType, db.Settings)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.Settings = settingsMap
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DatabaseResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DatabaseResourceModel
	var state DatabaseResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Check if disabled state changed
	if data.Disabled.ValueBool() != state.Disabled.ValueBool() {
		err := r.client.ToggleDatabase(data.Name.ValueString(), data.Disabled.ValueBool())
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Updating Database",
				"Could not toggle database state: "+err.Error(),
			)
			return
		}
	}

	// Note: Settings updates would require saving the full database record
	// This is more complex and may be implemented in a future version

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DatabaseResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DatabaseResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Delete the database (hard delete)
	err := r.client.DeleteDatabase(data.Name.ValueString(), true)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting Database",
			"Could not delete database: "+err.Error(),
		)
		return
	}
}

func (r *DatabaseResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
