package resources

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strconv"
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
				Description: "Password for the certificate. For generated certificates, this is the passphrase for the PFX. For uploaded certificates, this is the password to decrypt the PFX. Stored in state (sensitive) so downstream resources can reference it; the server does not return the password, so it is preserved from state across reads.",
				Optional:    true,
				Computed:    true,
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
				Description: "Number of days until the generated certificate expires. Only used when generating certificates; the server does not return this value, so it is preserved from state.",
				Optional:    true,
				Computed:    true,
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

	// Ensure Optional+Computed fields are known. Password defaults to empty
	// string (no password); ExpireAfterDays stays null if unspecified.
	if data.Password.IsUnknown() || data.Password.IsNull() {
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

	// Ensure Optional+Computed fields are known.
	if data.Password.IsUnknown() || data.Password.IsNull() {
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

	// password and expire_after_days are Optional+Computed: if the plan does
	// not specify a value, preserve whatever state already had.
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

// importParams holds the parsed values from a certificate import ID.
type importParams struct {
	Thumbprint      string
	Password        string
	HasPassword     bool
	ExpireAfterDays int64
	HasExpire       bool
	PFXPath         string
	PEMPath         string
}

// parseImportID parses a certificate import ID of the form:
//
//	<thumbprint>[:key=value]*
//
// where supported keys are: password, expire_after_days, pfx, pem.
// Values may be URL-encoded (e.g. "p%40ss" for "p@ss") to embed ':' or '='.
func parseImportID(id string) (*importParams, error) {
	if id == "" {
		return nil, fmt.Errorf("import ID is empty")
	}
	parts := strings.Split(id, ":")
	thumbprint := parts[0]
	if thumbprint == "" {
		return nil, fmt.Errorf("thumbprint is required")
	}
	p := &importParams{Thumbprint: thumbprint}
	for _, kv := range parts[1:] {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("expected key=value in segment %q", kv)
		}
		key := kv[:eq]
		raw := kv[eq+1:]
		val, err := url.QueryUnescape(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to decode value for key %q: %w", key, err)
		}
		switch key {
		case "password":
			p.Password = val
			p.HasPassword = true
		case "expire_after_days":
			n, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("expire_after_days must be an integer, got %q", val)
			}
			p.ExpireAfterDays = n
			p.HasExpire = true
		case "pfx":
			p.PFXPath = val
		case "pem":
			p.PEMPath = val
		default:
			return nil, fmt.Errorf("unknown import key %q (expected: password, expire_after_days, pfx, pem)", key)
		}
	}
	return p, nil
}

func (r *CertificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID formats:
	//   <thumbprint>
	//   <thumbprint>:password=<val>
	//   <thumbprint>:pfx=<path>:password=<val>
	//   <thumbprint>:pem=<path>
	//   <thumbprint>:expire_after_days=<n>:...
	//
	// Values may be URL-encoded if they contain ':' or '='.
	//
	// If a pfx or pem file path is supplied, the provider reads the file,
	// verifies its thumbprint matches the import ID, and populates
	// pfx_base64 / pem_base64 / not_after in state. Without a file path,
	// those fields stay null (the server does not re-emit the private key).
	params, err := parseImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("thumbprint"), params.Thumbprint)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if params.HasPassword {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("password"), params.Password)...)
	}
	if params.HasExpire {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("expire_after_days"), params.ExpireAfterDays)...)
	}

	if params.PFXPath != "" {
		if err := r.importFromPFX(ctx, params, resp); err != nil {
			resp.Diagnostics.AddError("Failed to import from PFX", err.Error())
			return
		}
	}
	if params.PEMPath != "" {
		if err := r.importFromPEM(ctx, params, resp); err != nil {
			resp.Diagnostics.AddError("Failed to import from PEM", err.Error())
			return
		}
	}
}

// certImportMaterial is the verified cert payload that an import file
// contributes to state.
type certImportMaterial struct {
	PFXBase64 string
	PEMBase64 string
	NotAfter  string
}

// extractFromPFX parses a PFX blob, verifies its thumbprint matches the
// expected value, and returns the base64-encoded PFX + a synthesized PEM
// + NotAfter in RFC3339.
func extractFromPFX(pfxData []byte, password, expectedThumbprint string) (*certImportMaterial, error) {
	info, err := client.ExtractCertInfoFromPFX(pfxData, password)
	if err != nil {
		return nil, fmt.Errorf("parse PFX: %w", err)
	}
	if !strings.EqualFold(info.Thumbprint, expectedThumbprint) {
		return nil, fmt.Errorf("thumbprint mismatch: PFX file contains %s, import ID has %s", info.Thumbprint, expectedThumbprint)
	}
	m := &certImportMaterial{
		PFXBase64: base64.StdEncoding.EncodeToString(pfxData),
		NotAfter:  info.NotAfter.UTC().Format(time.RFC3339),
	}
	if pemData, err := client.PFXToPEM(pfxData, password); err == nil {
		m.PEMBase64 = base64.StdEncoding.EncodeToString(pemData)
	}
	return m, nil
}

// extractFromPEM parses a PEM blob, verifies its thumbprint matches the
// expected value, and returns the base64-encoded PEM + NotAfter in RFC3339.
func extractFromPEM(pemData []byte, expectedThumbprint string) (*certImportMaterial, error) {
	cert, err := client.ParseCertFromPEM(pemData)
	if err != nil {
		return nil, fmt.Errorf("parse PEM: %w", err)
	}
	got := client.CalculateThumbprint(cert)
	if !strings.EqualFold(got, expectedThumbprint) {
		return nil, fmt.Errorf("thumbprint mismatch: PEM file contains %s, import ID has %s", got, expectedThumbprint)
	}
	return &certImportMaterial{
		PEMBase64: base64.StdEncoding.EncodeToString(pemData),
		NotAfter:  cert.NotAfter.UTC().Format(time.RFC3339),
	}, nil
}

func (r *CertificateResource) importFromPFX(ctx context.Context, p *importParams, resp *resource.ImportStateResponse) error {
	pfxData, err := os.ReadFile(p.PFXPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", p.PFXPath, err)
	}
	m, err := extractFromPFX(pfxData, p.Password, p.Thumbprint)
	if err != nil {
		return err
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pfx_base64"), m.PFXBase64)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("not_after"), m.NotAfter)...)
	// Only fill pem_base64 if the caller didn't also supply a PEM file
	// (which will be processed afterwards and is authoritative).
	if p.PEMPath == "" && m.PEMBase64 != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pem_base64"), m.PEMBase64)...)
	}
	return nil
}

func (r *CertificateResource) importFromPEM(ctx context.Context, p *importParams, resp *resource.ImportStateResponse) error {
	pemData, err := os.ReadFile(p.PEMPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", p.PEMPath, err)
	}
	m, err := extractFromPEM(pemData, p.Thumbprint)
	if err != nil {
		return err
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pem_base64"), m.PEMBase64)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("not_after"), m.NotAfter)...)
	return nil
}
