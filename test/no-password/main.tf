terraform {
  required_providers {
    ravendb = {
      source = "zerotomvp/ravendb-admin"
    }
  }
}

provider "ravendb" {
  url                  = "https://localhost:8443"
  certificate_path     = "/tmp/ravendb-secure/certs/server.pem"
  insecure_skip_verify = true
}

# Test: Generate certificate WITHOUT a password
# This should work but currently fails with:
# "Provider produced inconsistent result after apply"
resource "ravendb_certificate" "no_password_cert" {
  name               = "test-no-password"
  # NOTE: password is intentionally omitted
  security_clearance = "ValidUser"
  expire_after_days  = 30
}

output "thumbprint" {
  value = ravendb_certificate.no_password_cert.thumbprint
}

output "has_pfx" {
  value     = ravendb_certificate.no_password_cert.pfx_base64 != ""
  sensitive = true
}
