terraform {
  required_providers {
    ravendb = {
      source = "zerotomvp/ravendb-admin"
    }
  }
}

# For certificate generation, you need a secured RavenDB instance
provider "ravendb" {
  url                  = "https://localhost:8443"
  certificate_path     = "/path/to/admin-client.pem"
  insecure_skip_verify = true
}

# Generate a new client certificate
resource "ravendb_certificate" "app_client" {
  name               = "my-app-client"
  password           = "secure-passphrase"
  security_clearance = "ValidUser"
  expire_after_days  = 365

  permissions = {
    "my-database" = "ReadWrite"
  }
}

# Upload an existing certificate
# resource "ravendb_certificate" "existing" {
#   name               = "existing-client"
#   certificate        = filebase64("client.pfx")
#   password           = "pfx-password"
#   security_clearance = "ValidUser"
#
#   permissions = {
#     "my-database" = "ReadWrite"
#   }
# }

# Output the certificate details
output "certificate_thumbprint" {
  value = ravendb_certificate.app_client.thumbprint
}

output "certificate_expiry" {
  value = ravendb_certificate.app_client.not_after
}

# Save the generated PFX to a file (sensitive)
resource "local_sensitive_file" "certificate_pfx" {
  content_base64 = ravendb_certificate.app_client.pfx_base64
  filename       = "${path.module}/generated-client.pfx"
}
