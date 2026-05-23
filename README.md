# Terraform Provider for RavenDB Administration

A Terraform provider for managing [RavenDB](https://ravendb.net/) 7.x server administration — databases and client certificates.

## Features

- **Database Management** — Create, update, and delete databases with configurable replication, encryption, and settings
- **Certificate Management** — Generate new client certificates or upload existing ones, with configurable security clearance and per-database permissions
- **Data Sources** — List databases and look up certificates by thumbprint

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.0
- RavenDB 7.x server

## Usage

```hcl
terraform {
  required_providers {
    ravendb = {
      source  = "zerotomvp/ravendb-admin"
      version = "~> 0.1"
    }
  }
}

# Unsecured connection (development only)
provider "ravendb" {
  url = "http://localhost:8080"
}

# Secured connection with client certificate
provider "ravendb" {
  url                  = "https://ravendb.example.com:443"
  certificate_path     = "/path/to/client.pfx"
  certificate_password = "optional-password"
  insecure_skip_verify = true  # For self-signed certs
}
```

### Databases

```hcl
resource "ravendb_database" "app" {
  name               = "my-database"
  replication_factor = 1
  disabled           = false
  encrypted          = false
}
```

### Certificates

```hcl
# Generate a new client certificate
resource "ravendb_certificate" "app_client" {
  name               = "my-app-client"
  password           = "certificate-passphrase"
  security_clearance = "ValidUser"  # ValidUser, Operator, ClusterAdmin
  expire_after_days  = 365

  permissions = {
    "my-database" = "ReadWrite"  # Read, ReadWrite, Admin
  }
}

# Upload an existing certificate
resource "ravendb_certificate" "existing" {
  name               = "existing-client"
  certificate        = filebase64("client.pfx")
  password           = "pfx-password"
  security_clearance = "ValidUser"

  permissions = {
    "my-database" = "ReadWrite"
  }
}

# Access generated certificate data
output "thumbprint" {
  value = ravendb_certificate.app_client.thumbprint
}

output "pfx_base64" {
  value     = ravendb_certificate.app_client.pfx_base64
  sensitive = true
}
```

### Data Sources

```hcl
# List all databases
data "ravendb_databases" "all" {}

output "database_names" {
  value = [for db in data.ravendb_databases.all.databases : db.name]
}

# Look up a certificate by thumbprint
data "ravendb_certificate" "my_cert" {
  thumbprint = "ABC123..."
}
```

> **Reading data sources after creating/modifying resources in the same apply**
>
> Terraform evaluates data sources at plan time by default. If you read
> `ravendb_databases` (or any other data source) and also create or modify
> resources that affect what the data source returns *in the same apply*,
> the data source's result will reflect the pre-apply state. The drift
> corrects itself on the next plan.
>
> To force the data source to be re-read after the resource changes, add
> `depends_on`:
>
> ```hcl
> resource "ravendb_database" "test" {
>   name               = "tf-review-db"
>   replication_factor = 1
> }
>
> data "ravendb_databases" "all" {
>   depends_on = [ravendb_database.test]
> }
> ```

## Resources Reference

### ravendb_database

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Database name (immutable after creation) |
| `replication_factor` | number | No | Number of replicas (default: 1) |
| `disabled` | bool | No | Whether database is disabled (default: false) |
| `encrypted` | bool | No | Whether database is encrypted (default: false) |
| `settings` | map(string) | No | Database settings |

### ravendb_certificate

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Certificate name |
| `certificate` | string | No | Base64-encoded PFX (upload mode) |
| `password` | string | No | Certificate password (omit for passwordless) |
| `security_clearance` | string | No | Security level (default: ValidUser) |
| `permissions` | map(string) | No | Database access permissions |
| `expire_after_days` | number | No | Days until expiration (generate mode) |
| `thumbprint` | string | Computed | Certificate thumbprint |
| `pfx_base64` | string | Computed | Generated PFX (sensitive) |
| `pem_base64` | string | Computed | Generated PEM (sensitive) |
| `not_after` | string | Computed | Expiration date |

#### Importing certificates

The minimum import ID is the certificate thumbprint:

```bash
terraform import ravendb_certificate.app_client <THUMBPRINT>
```

A bare import only recovers metadata the server can return (name, security
clearance, permissions, `not_after`). The `password`, `expire_after_days`,
`pfx_base64`, and `pem_base64` attributes will be null in state because the
server never re-emits private keys or write-time inputs.

If you still have the original cert file locally, supply it during import to
populate the rest of state:

```bash
# Import with the original PFX so pfx_base64 / pem_base64 / not_after land in state
terraform import 'ravendb_certificate.app_client' \
  '<THUMBPRINT>:pfx=/path/to/cert.pfx:password=<password>:expire_after_days=365'

# Or import with a PEM
terraform import 'ravendb_certificate.app_client' \
  '<THUMBPRINT>:pem=/path/to/cert.pem'
```

Supported keys: `password`, `expire_after_days`, `pfx`, `pem`. Values containing
`:` or `=` must be URL-encoded (e.g. `password=p%40ss%3Aword` for `p@ss:word`).
The provider reads the file, verifies the thumbprint embedded in it matches
the one you supplied, and refuses the import on mismatch.

If you no longer have the cert file, you cannot recover `pfx_base64` /
`pem_base64`. The only way to obtain fresh material is to rotate (replace)
the resource — the RavenDB server only emits the private key once, at
generation time.

## Data Sources Reference

### ravendb_databases

Returns a list of all databases with their status.

| Attribute | Type | Description |
|-----------|------|-------------|
| `databases` | list | List of database objects |
| `databases[].name` | string | Database name |
| `databases[].disabled` | bool | Whether disabled |
| `databases[].encrypted` | bool | Whether encrypted |

### ravendb_certificate

Look up a certificate by thumbprint.

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `thumbprint` | string | Yes | Certificate thumbprint to look up |
| `name` | string | Computed | Certificate name |
| `security_clearance` | string | Computed | Security clearance level |
| `not_after` | string | Computed | Expiration date |
| `permissions` | map(string) | Computed | Database permissions |

## Changelog

The section below is synced automatically from
[GitHub Releases](https://github.com/zerotomvp/terraform-provider-ravendb-admin/releases)
on every release publish — each release body is generated by goreleaser from
the commits since the previous tag, grouped by conventional-commit type. To
edit a past release's notes, update its GitHub release body and re-run the
`Sync README changelog` workflow.

<!-- changelog:start -->
_Not yet generated. Will populate on the next release publish._
<!-- changelog:end -->

## Development

See [TESTING.md](TESTING.md) for testing instructions.

## License

MIT
