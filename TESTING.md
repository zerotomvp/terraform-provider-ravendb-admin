# Testing the RavenDB Terraform Provider

This document describes how to set up test environments and run tests for the provider.

## Test Environments

The provider requires two RavenDB instances for complete testing:

1. **Unsecured instance** - For basic database operations
2. **Secured instance** - For certificate generation (requires HTTPS with server certificate)

## Setting Up Test Instances

### 1. Unsecured Instance (Development)

If you already have a RavenDB instance running:

```bash
# Verify it's accessible
curl http://localhost:8080/build/version
```

Or start one with Docker:

```bash
docker run -d --name ravendb-unsecure \
  -p 8080:8080 \
  -e RAVEN_Security_UnsecuredAccessAllowed=PublicNetwork \
  -e RAVEN_Setup_Mode=None \
  -e RAVEN_License_Eula_Accepted=true \
  ravendb/ravendb:7.1-latest
```

### 2. Secured Instance (For Certificate Testing)

Setting up a secured instance requires generating a server certificate:

```bash
# Create directories
mkdir -p /tmp/ravendb-secure/{data,certs}

# Create OpenSSL config for proper key usage
cat > /tmp/ravendb-secure/certs/openssl.cnf << 'EOF'
[req]
default_bits = 4096
prompt = no
default_md = sha256
distinguished_name = dn
x509_extensions = v3_ca
req_extensions = v3_req

[dn]
CN = localhost

[v3_ca]
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always,issuer
basicConstraints = critical, CA:true
keyUsage = critical, digitalSignature, cRLSign, keyCertSign

[v3_req]
basicConstraints = CA:FALSE
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth, clientAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
IP.1 = 127.0.0.1
EOF

# Generate server certificate
openssl req -x509 -newkey rsa:4096 \
  -keyout /tmp/ravendb-secure/certs/server.key \
  -out /tmp/ravendb-secure/certs/server.crt \
  -days 365 -nodes \
  -config /tmp/ravendb-secure/certs/openssl.cnf \
  -extensions v3_req

# Create PFX for RavenDB
openssl pkcs12 -export \
  -out /tmp/ravendb-secure/certs/server.pfx \
  -inkey /tmp/ravendb-secure/certs/server.key \
  -in /tmp/ravendb-secure/certs/server.crt \
  -passout pass:ravendb

# Create combined PEM for client authentication
cat /tmp/ravendb-secure/certs/server.key \
    /tmp/ravendb-secure/certs/server.crt > /tmp/ravendb-secure/certs/server.pem

# Set permissions
chmod 644 /tmp/ravendb-secure/certs/server.pfx
chmod 644 /tmp/ravendb-secure/certs/server.pem

# Start secured RavenDB
docker run -d --name ravendb-secure \
  -p 8443:443 \
  -p 38889:38888 \
  -e RAVEN_Security_UnsecuredAccessAllowed=PrivateNetwork \
  -e RAVEN_Setup_Mode=None \
  -e RAVEN_License_Eula_Accepted=true \
  -e RAVEN_ServerUrl=https://0.0.0.0:443 \
  -e RAVEN_PublicServerUrl=https://localhost:8443 \
  -e RAVEN_ServerUrl_Tcp=tcp://0.0.0.0:38888 \
  -e RAVEN_PublicServerUrl_Tcp=tcp://localhost:38889 \
  -e RAVEN_Security_Certificate_Path=/opt/RavenDB/Server/certs/server.pfx \
  -e RAVEN_Security_Certificate_Password=ravendb \
  -v /tmp/ravendb-secure/data:/opt/RavenDB/Server/RavenData \
  -v /tmp/ravendb-secure/certs:/opt/RavenDB/Server/certs \
  ravendb/ravendb:7.1-latest
```

Verify the secured instance:

```bash
# Should require client certificate
curl -k https://localhost:8443/build/version

# With client certificate
curl -sk --cert /tmp/ravendb-secure/certs/server.crt \
         --key /tmp/ravendb-secure/certs/server.key \
         https://localhost:8443/build/version
```

## Building the Provider

**IMPORTANT**: The binary name must match the provider name suffix. Since the provider is `zerotomvp/ravendb-admin`, the binary must be named `terraform-provider-ravendb-admin`.

```bash
cd terraform-provider-ravendb
go build -o terraform-provider-ravendb-admin
```

## Local Development Setup

For local development, use Terraform's `dev_overrides` feature instead of copying binaries to plugin directories.

1. Build the provider:
   ```bash
   cd terraform-provider-ravendb
   go build -o terraform-provider-ravendb-admin
   ```

2. Create or update `~/.terraformrc`:
   ```hcl
   provider_installation {
     dev_overrides {
       "zerotomvp/ravendb-admin" = "/absolute/path/to/terraform-provider-ravendb"
     }
     direct {}
   }
   ```

   Example for this repository:
   ```hcl
   provider_installation {
     dev_overrides {
       "zerotomvp/ravendb-admin" = "/home/georgiosd/src/infrastructure-ravendb-terraform/terraform-provider-ravendb"
     }
     direct {}
   }
   ```

3. With `dev_overrides` configured:
   - No need to run `terraform init`
   - Terraform uses the local binary directly
   - You'll see a warning: "Provider development overrides are in effect" (expected)
   - After making code changes, just rebuild and re-run `terraform plan/apply`

## Running Tests

### Test 1: Unsecured Instance - Database CRUD

```bash
cd test/

# Create test configuration
cat > main.tf << 'EOF'
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

data "ravendb_databases" "all" {}

resource "ravendb_database" "test" {
  name               = "terraform-test-db"
  replication_factor = 1
}

output "existing_databases" {
  value = [for db in data.ravendb_databases.all.databases : db.name]
}

output "created_database" {
  value = ravendb_database.test.name
}
EOF

# Apply (no terraform init needed with dev_overrides)
terraform plan
terraform apply -auto-approve

# Verify database was created
curl -s http://localhost:8080/databases | jq '.Databases[].Name'

# Clean up
terraform destroy -auto-approve
```

### Test 2: Secured Instance - Database and Certificates

```bash
cd test/secured/

# Create test configuration
cat > main.tf << 'EOF'
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
  name               = "terraform-secured-test"
  replication_factor = 1
}

resource "ravendb_certificate" "app_client" {
  name               = "terraform-test-client"
  password           = "test-password"
  security_clearance = "ValidUser"
  expire_after_days  = 30

  permissions = {
    "terraform-secured-test" = "ReadWrite"
  }

  depends_on = [ravendb_database.test]
}

output "certificate_thumbprint" {
  value = ravendb_certificate.app_client.thumbprint
}
EOF

# Apply (no terraform init needed with dev_overrides)
terraform plan
terraform apply -auto-approve

# Clean up
terraform destroy -auto-approve
```

## Manual API Testing

### Database Operations

```bash
# List databases
curl -s http://localhost:8080/databases | jq '.Databases[].Name'

# Create database
curl -X PUT "http://localhost:8080/admin/databases?name=test-db&replicationFactor=1" \
  -H "Content-Type: application/json" \
  -d '{"DatabaseName": "test-db"}'

# Get database record
curl -s "http://localhost:8080/admin/databases?name=test-db" | jq

# Delete database
curl -X DELETE http://localhost:8080/admin/databases \
  -H "Content-Type: application/json" \
  -d '{"HardDelete": true, "DatabaseNames": ["test-db"]}'
```

### Certificate Operations (Secured Instance)

```bash
CERT_OPTS="--cert /tmp/ravendb-secure/certs/server.crt --key /tmp/ravendb-secure/certs/server.key"

# List certificates
curl -sk $CERT_OPTS https://localhost:8443/admin/certificates | jq

# Generate certificate (POST with form data)
curl -sk $CERT_OPTS -X POST \
  "https://localhost:8443/admin/certificates?operationId=1" \
  -F 'Options={"Name":"test-cert","SecurityClearance":"ValidUser","Permissions":{}}' \
  -o test-cert.zip

# Delete certificate by thumbprint
curl -sk $CERT_OPTS -X DELETE \
  "https://localhost:8443/admin/certificates?thumbprint=ABC123..."
```

## Known Issues

### Docker Networking

When running RavenDB in Docker with `PublicServerUrl=https://localhost:8443`, the container cannot reach `localhost` on the host. This causes issues when creating databases with replication, as RavenDB tries to verify the cluster topology.

**Workaround**: Use host networking or configure the PublicServerUrl to use the Docker host's IP address.

### Certificate Generation

Certificate generation only works on secured RavenDB instances. The server needs its own certificate to sign client certificates.

## Cleanup

```bash
# Stop and remove containers
docker rm -f ravendb-unsecure ravendb-secure

# Remove test data
rm -rf /tmp/ravendb-secure
rm -rf test/.terraform* test/terraform.tfstate*
rm -rf test/secured/.terraform* test/secured/terraform.tfstate*
```
