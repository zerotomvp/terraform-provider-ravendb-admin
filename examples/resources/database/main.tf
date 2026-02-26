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

# Create a simple database
resource "ravendb_database" "example" {
  name               = "terraform-test-db"
  replication_factor = 1
}

# Create a database with custom settings
resource "ravendb_database" "with_settings" {
  name               = "terraform-test-db-2"
  replication_factor = 1
  disabled           = false
  encrypted          = false
}

# Output the database names
output "database_names" {
  value = [
    ravendb_database.example.name,
    ravendb_database.with_settings.name,
  ]
}
