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

resource "ravendb_database" "test" {
  name = "test-cert-bug"
  # Don't specify replication_factor for single-node test setup
}

resource "ravendb_certificate" "test_cert" {
  name               = "test-certificate"
  password           = "test123"
  security_clearance = "ValidUser"
  expire_after_days  = 30
  
  permissions = {
    "test-cert-bug" = "Admin"
  }

  depends_on = [ravendb_database.test]
}

output "thumbprint" {
  value = ravendb_certificate.test_cert.thumbprint
}
