terraform {
  required_providers {
    ravendb = {
      source = "zerotomvp/ravendb-admin"
    }
  }
}

# Unsecured connection (for development/testing)
provider "ravendb" {
  url = "http://localhost:8080"
}

# Secured connection with client certificate
# provider "ravendb" {
#   url                  = "https://localhost:8443"
#   certificate_path     = "/path/to/client.pem"
#   certificate_password = "optional-password"
#   insecure_skip_verify = true  # Only for self-signed certs
# }
