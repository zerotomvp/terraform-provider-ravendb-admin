terraform {
  required_providers {
    ravendb = {
      source = "zerotomvp/ravendb-admin"
    }
  }
}

provider "ravendb" {
  url = "http://localhost:8080"
}

# List all databases
data "ravendb_databases" "all" {}

output "all_databases" {
  value = data.ravendb_databases.all.databases
}

# Look up a specific certificate by thumbprint
# data "ravendb_certificate" "my_cert" {
#   thumbprint = "ABC123..."
# }
#
# output "certificate_info" {
#   value = {
#     name              = data.ravendb_certificate.my_cert.name
#     security_clearance = data.ravendb_certificate.my_cert.security_clearance
#     expires           = data.ravendb_certificate.my_cert.not_after
#   }
# }
