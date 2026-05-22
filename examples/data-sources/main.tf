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

# List all databases.
#
# Note: data sources are read at plan time. If you also create/modify
# `ravendb_database` resources in the same apply, this data source will
# reflect the pre-apply state until the next plan. Add `depends_on` to
# force a re-read after resource changes:
#
#   data "ravendb_databases" "all" {
#     depends_on = [ravendb_database.test]
#   }
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
