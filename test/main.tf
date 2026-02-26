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

# List existing databases
data "ravendb_databases" "all" {}

output "existing_databases" {
  value = [for db in data.ravendb_databases.all.databases : db.name]
}

# Create a test database
resource "ravendb_database" "test" {
  name               = "terraform-provider-test"
  replication_factor = 1
}

output "created_database" {
  value = ravendb_database.test.name
}
