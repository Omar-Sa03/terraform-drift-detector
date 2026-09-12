# tdd (Terraform Drift Detector)

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

tdd is a lightweight, read-only drift detection tool for Terraform. It checks your cloud infrastructure by comparing your Terraform state against live cloud APIs directly, without running terraform plan or acquiring state locks.

## Why tdd?

Running `terraform plan` continuously to detect drift has clear disadvantages:

* Slow execution: Terraform must initialize backend plugins and download large provider binaries.
* State locking: Generating a plan acquires write locks on the state file, which can block active CI/CD pipelines and developer deployments.
* Broad permissions: Terraform often requires broader permissions than necessary just to plan.

`tdd` parses your `terraform.tfstate` directly (from a local file or remote S3 backend) and queries live resources using read-only cloud SDK calls. Scans complete in seconds without touching your deployment workflows.

## Features

* Drift detection: Spots out-of-band deletions, modified resource attributes, and tag changes.
* Accurate comparison: Compares only attributes tracked by live cloud fetchers, avoiding false positives from computed defaults and internal Terraform metadata.
* Multiple output formats: Clean terminal tables, JSON output for CI/CD scripting, and an embedded web dashboard.
* Flexible scheduling: Includes a built-in cron scheduler for background scanning.
* Single binary: Compiles down to a standalone binary with zero runtime dependencies.

## Installation

Requires Go 1.22 or higher.

```bash
git clone https://github.com/Omar-Sa03/terraform-drift-detector.git
cd terraform-drift-detector

# Linux and macOS
go build -o tdd ./cmd/tdd

# Windows
go build -o tdd.exe ./cmd/tdd
```

## AWS Authentication

tdd uses the standard AWS SDK credential chain. You can authenticate using standard AWS CLI profiles or environment variables:

```bash
# Option 1: Standard AWS CLI configuration
aws configure

# Option 2: Environment variables
export AWS_REGION=eu-west-3
export AWS_ACCESS_KEY_ID="AKIA..."
export AWS_SECRET_ACCESS_KEY="..."
```

## Usage

### Ad-hoc scan

Run a scan against a local state file:

```bash
./tdd scan --state ../infra/terraform.tfstate
```

Example terminal output:

```text
Scanned: 2026-09-12T09:28:37Z
State:   terraform.tfstate
Summary: 1 expected, 1 live, 2 drifts (deleted=0 modified=1 tags=1 created=0)

KIND         ADDRESS                  PATH        BEFORE                                 AFTER
modified     aws_s3_bucket.my_bucket  versioning  [map[enabled:false mfa_delete:false]]  [map[enabled:true mfa_delete:false]]
tag_changed  aws_s3_bucket.my_bucket  tags.env                                           develop
```

### Remote S3 state backend

Scan state stored in an S3 bucket:

```bash
./tdd scan --backend-s3 my-tf-bucket/prod/terraform.tfstate --region eu-west-3
```

### JSON output for automation

Export structured JSON for alerting or pipeline scripts:

```bash
./tdd scan --state terraform.tfstate --json --out .tdd/report.json
```

### Scheduled background scans

Run tdd on a recurring schedule (for example, every 15 minutes):

```bash
./tdd scan --state terraform.tfstate --schedule "*/15 * * * *"
```

### Web dashboard

Launch the local web dashboard to view drift reports and trigger scans on demand:

```bash
./tdd serve --state terraform.tfstate --listen 127.0.0.1:8080
```

Open http://127.0.0.1:8080 in your browser.

## Supported AWS Resources (v1)

| Terraform Resource | Live Attributes Monitored |
|---|---|
| `aws_s3_bucket` | Bucket existence, tags, versioning status (Enabled / Suspended) |
| `aws_instance` | AMI, instance type, subnet ID, availability zone, security groups, tags |
| `aws_security_group` | Group name, description, VPC ID, tags |
| `aws_vpc` | CIDR block, tags |
| `aws_subnet` | CIDR block, VPC ID, availability zone, tags |
| `aws_iam_role` | Role name, ARN, assume role policy document, tags |

Resources in the state file that are not yet implemented are skipped with a warning message. Data sources are ignored.

## Architecture

```text
  +------------------------+       +------------------------+
  |  Local / Remote State  |       |     Live Cloud API     |
  |    (terraform.tfstate) |       |   (AWS SDK Go v2)      |
  +-----------+------------+       +-----------+------------+
              |                                |
              v                                v
       [State Loader]                   [Cloud Fetcher]
              |                                |
              +---------------+----------------+
                              |
                              v
                     [Comparison Engine]
                              |
               +--------------+--------------+
               |              |              |
               v              v              v
         Terminal Table      JSON      Web Dashboard
```

* `internal/state`: Parses Terraform state formats (v3 and v4), with local file and S3 loaders.
* `internal/provider`: Provider interfaces and cloud fetchers (AWS implementation with pluggable stubs for Azure and GCP).
* `internal/compare`: Comparison engine that normalizes data types, filters ignored metadata, and calculates diffs.
* `internal/http` and `web`: Embedded HTTP server and web dashboard for interactive review.

## Tests

Run the test suite:

```bash
go test ./...
```

## Contributing

To add support for additional AWS resources or cloud providers:

1. Implement the `provider.Fetcher` interface in `internal/provider/`.
2. Map live SDK fields to corresponding Terraform attribute names.
3. Add unit tests with mock API responses.
4. Submit a pull request.

## License

This project is licensed under the MIT License.
