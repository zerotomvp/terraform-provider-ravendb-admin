package resources

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/zerotomvp/terraform-provider-ravendb-admin/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &CertificateResource{}
var _ resource.ResourceWithImportState = &CertificateResource{}

func NewCertificateResource() resource.Resource {
	return &CertificateResource{}
}

// CertificateResource defines the resource implementation.
type CertificateResource struct {
	client *client.Client
}

// CertificateResourceModel describes the resource data model.
type CertificateResourceModel struct {
	Name              types.String `tfsdk:"name"`
	Certificate       types.String `tfsdk:"certificate"`
	Password          types.String `tfsdk:"password"`
	SecurityClearance types.String `tfsdk:"security_clearance"`
	Permissions       types.Map    `tfsdk:"permissions"`
	ExpireAfterDays   types.Int64  `tfsdk:"expire_after_days"`

	// Computed attributes
	Thumbprint types.String `tfsdk:"thumbprint"`
	PFXBase64  types.String `tfsdk:"pfx_base64"`
	PEMBase64  types.String `tfsdk:"pem_base64"`
	NotAfter   types.String `tfsdk:"not_after"`
}

func (r *CertificateResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_certificate"
}

func (r *CertificateResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a RavenDB client certificate. Can either generate a new certificate or upload an existing one.",

		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "The name of the certificate.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"certificate": schema.StringAttribute{
				Description: "Base64-encoded certificate (PFX or PEM) to upload. If not provided, a new certificate will be generated.",
				Optional:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"password": schema.StringAttribute{
				Description: "Password for the certificate. For generated certificates, this is the passphrase for the PFX. For uploaded certificates, this is the password to decrypt the PFX.",
				Optional:    true,
				Computed:    true, // Not returned by API, preserve from state
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"security_clearance": schema.StringAttribute{
				Description: "Security clearance level: ValidUser, Operator, or ClusterAdmin.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("ValidUser"),
			},
			"permissions": schema.MapAttribute{
				Description: "Database permissions. Keys are database names, values are access levels (Read, ReadWrite, Admin).",
				Optional:    true,
				ElementType: types.StringType,
			},
			"expire_after_days": schema.Int64Attribute{
				Description: "Number of days until the generated certificate expires. Default is 1825 (5 years). Only used when generating certificates.",
				Optional:    true,
				Computed:    true, // Not returned by API, preserve from state
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},

			// Computed attributes
			"thumbprint": schema.StringAttribute{
				Description: "The thumbprint of the certificate.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"pfx_base64": schema.StringAttribute{
				Description: "The generated PFX certificate (base64-encoded). Only populated when generating a certificate.",
				Computed:    true,
				Sensitive:   true,
			},
			"pem_base64": schema.StringAttribute{
				Description: "The generated PEM certificate (base64-encoded). Only populated when generating a certificate.",
				Computed:    true,
				Sensitive:   true,
			},
			"not_after": schema.StringAttribute{
				Description: "Certificate expiration date.",
				Computed:    true,
			},
		},
	}
}

func (r *CertificateResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *CertificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CertificateResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Convert permissions map
	var permissions map[string]string
	if !data.Permissions.IsNull() {
		permissions = make(map[string]string)
		resp.Diagnostics.Append(data.Permissions.ElementsAs(ctx, &permissions, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if data.Certificate.IsNull() || data.Certificate.ValueString() == "" {
		// Generate mode
		r.createGenerated(ctx, &data, permissions, resp)
	} else {
		// Upload mode
		r.createUploaded(ctx, &data, permissions, resp)
	}
}

func (r *CertificateResource) createGenerated(ctx context.Context, data *CertificateResourceModel, permissions map[string]string, resp *resource.CreateResponse) {
	// Calculate expiration date
	var notAfter string
	if !data.ExpireAfterDays.IsNull() && !data.ExpireAfterDays.IsUnknown() {
		days := data.ExpireAfterDays.ValueInt64()
		expireTime := time.Now().AddDate(0, 0, int(days))
		notAfter = expireTime.Format(time.RFC3339)
	}

	// Get password value, defaulting to empty string if null/unknown
	password := ""
	if !data.Password.IsNull() && !data.Password.IsUnknown() {
		password = data.Password.ValueString()
	}

	genReq := client.GenerateCertificateRequest{
		Name:              data.Name.ValueString(),
		Password:          password,
		SecurityClearance: data.SecurityClearance.ValueString(),
		NotAfter:          notAfter,
		Permissions:       permissions,
	}

	result, err := r.client.GenerateCertificate(genReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Generating Certificate",
			"Could not generate certificate: "+err.Error(),
		)
		return
	}

	// Store the generated certificate data
	data.PFXBase64 = types.StringValue(base64.StdEncoding.EncodeToString(result.PFX))
	data.PEMBase64 = types.StringValue(base64.StdEncoding.EncodeToString(result.PEM))

	// Use thumbprint and NotAfter extracted directly from the generated certificate
	if result.Thumbprint == "" {
		resp.Diagnostics.AddError(
			"Error Extracting Certificate Info",
			"Certificate was generated but could not extract thumbprint from the PFX file.",
		)
		return
	}
	data.Thumbprint = types.StringValue(result.Thumbprint)
	data.NotAfter = types.StringValue(result.NotAfter)

	// Ensure computed fields are set to known values
	// This is required because these fields are marked as Computed and Terraform
	// requires all computed values to be known after apply.
	if data.Password.IsUnknown() || data.Password.IsNull() {
		// If no password was provided, set it to empty string (no password)
		data.Password = types.StringValue("")
	}
	if data.ExpireAfterDays.IsUnknown() {
		data.ExpireAfterDays = types.Int64Null()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *CertificateResource) createUploaded(ctx context.Context, data *CertificateResourceModel, permissions map[string]string, resp *resource.CreateResponse) {
	cert := client.Certificate{
		Name:              data.Name.ValueString(),
		Certificate:       data.Certificate.ValueString(),
		Password:          data.Password.ValueString(),
		SecurityClearance: data.SecurityClearance.ValueString(),
		Permissions:       permissions,
	}

	err := r.client.UploadCertificate(cert)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Uploading Certificate",
			"Could not upload certificate: "+err.Error(),
		)
		return
	}

	// Get the certificate from the server to retrieve computed values
	certs, err := r.client.ListCertificates()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Certificate",
			"Certificate was uploaded but could not read it back: "+err.Error(),
		)
		return
	}

	// Find the certificate by name
	found := false
	for _, c := range certs {
		if c.Name == data.Name.ValueString() {
			data.Thumbprint = types.StringValue(c.Thumbprint)
			data.NotAfter = types.StringValue(c.NotAfter)
			found = true
			break
		}
	}

	if !found {
		resp.Diagnostics.AddError(
			"Error Finding Certificate",
			"Certificate was uploaded but could not find it in the certificate list. This may indicate a server-side issue.",
		)
		return
	}

	// PFX and PEM are not populated for uploaded certificates
	data.PFXBase64 = types.StringNull()
	data.PEMBase64 = types.StringNull()

	// Ensure computed fields are set to known values
	// This is required because these fields are marked as Computed and Terraform
	// requires all computed values to be known after apply.
	// We must explicitly set these even if the user provided a value, to ensure
	// the state contains a known value (not unknown).
	if data.Password.IsUnknown() || data.Password.IsNull() {
		// If no password was provided, set it to empty string (no password)
		data.Password = types.StringValue("")
	}
	if data.ExpireAfterDays.IsUnknown() {
		data.ExpireAfterDays = types.Int64Null()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *CertificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CertificateResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get the certificate by thumbprint
	cert, err := r.client.GetCertificate(data.Thumbprint.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Certificate",
			"Could not read certificate: "+err.Error(),
		)
		return
	}

	if cert == nil {
		// Certificate was deleted outside of Terraform
		resp.State.RemoveResource(ctx)
		return
	}

	// Update state with values from API
	data.Name = types.StringValue(cert.Name)
	data.SecurityClearance = types.StringValue(cert.SecurityClearance)
	data.NotAfter = types.StringValue(cert.NotAfter)

	if cert.Permissions != nil && len(cert.Permissions) > 0 {
		permissionsMap, diags := types.MapValueFrom(ctx, types.StringType, cert.Permissions)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.Permissions = permissionsMap
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CertificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data CertificateResourceModel
	var state CertificateResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Convert permissions map
	var permissions map[string]string
	if !data.Permissions.IsNull() {
		permissions = make(map[string]string)
		resp.Diagnostics.Append(data.Permissions.ElementsAs(ctx, &permissions, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Update certificate metadata
	updateReq := client.UpdateCertificateRequest{
		Name:              data.Name.ValueString(),
		Thumbprint:        state.Thumbprint.ValueString(),
		SecurityClearance: data.SecurityClearance.ValueString(),
		Permissions:       permissions,
	}

	err := r.client.UpdateCertificate(updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Certificate",
			"Could not update certificate: "+err.Error(),
		)
		return
	}

	// Preserve computed values from state
	data.Thumbprint = state.Thumbprint
	data.PFXBase64 = state.PFXBase64
	data.PEMBase64 = state.PEMBase64
	data.NotAfter = state.NotAfter

	// Preserve write-only fields from plan (they're already in 'data' from req.Plan.Get)
	// But if they're null in plan, preserve from state
	if data.Password.IsNull() && !state.Password.IsNull() {
		data.Password = state.Password
	}
	if data.ExpireAfterDays.IsNull() && !state.ExpireAfterDays.IsNull() {
		data.ExpireAfterDays = state.ExpireAfterDays
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CertificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CertificateResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteCertificate(data.Thumbprint.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting Certificate",
			"Could not delete certificate: "+err.Error(),
		)
		return
	}
}

func (r *CertificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: "thumbprint" or "thumbprint:password"
	// This allows importing certificates with or without password
	parts := strings.Split(req.ID, ":")

	// Set thumbprint (always required)
	thumbprint := parts[0]
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("thumbprint"), thumbprint)...)

	// Set password if provided (optional)
	if len(parts) == 2 && parts[1] != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("password"), parts[1])...)
	}
	// If len(parts) == 1, no password provided - leave it unset in state

	// Validate format
	if len(parts) > 2 {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected format 'thumbprint' or 'thumbprint:password', got: %s", req.ID),
		)
	}
}
