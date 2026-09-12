# Terraform Drift Detector

Go CLI that compares Terraform state to live cloud APIs. It never runs `terraform plan` or `apply`.

## What it does

1. Reads expected resources from a local `terraform.tfstate` or an S3 backend object.
2. Fetches matching live metadata from cloud provider APIs (AWS in v1).
3. Normalizes both sides into a common resource model.
4. Reports **deleted** resources, **modified** attributes, and **tag** changes (optional **created**/unmanaged).
5. Prints a table, JSON, or a one-page dashboard. Scans can run once or on a cron schedule.

Azure and GCP adapters are registered as stubs so you can add API mappers later without changing the engine.

## Install

Requires Go 1.22+.

```bash
go build -o tdd ./cmd/tdd
```

On Windows:

```powershell
go build -o tdd.exe ./cmd/tdd
```

## AWS credentials

Uses the default AWS SDK chain: environment variables, shared config (`AWS_PROFILE`), or instance/container roles.

```bash
export AWS_REGION=us-east-1
# or: --region us-east-1
```

## Usage

On-demand table:

```bash
./tdd scan --state testdata/example.tfstate
```

JSON:

```bash
./tdd scan --state terraform.tfstate --json
```

Remote state:

```bash
./tdd scan --backend-s3 my-tf-state-bucket/env/terraform.tfstate --region us-east-1
```

Schedule (process stays running):

```bash
./tdd scan --state terraform.tfstate --schedule "*/30 * * * *"
```

Dashboard (last report + **Run scan**):

```bash
./tdd serve --state terraform.tfstate --listen 127.0.0.1:8080
```

Open `http://127.0.0.1:8080`. Reports are written to `.tdd/last-report.json` by default (`--out`).

`--unmanaged` also flags live objects returned by fetchers that are not in state (v1 AWS fetch is ID-driven from state, so this is mainly useful as you extend fetchers).

## Supported AWS types (v1)

| Terraform type       | API                          |
|----------------------|------------------------------|
| `aws_instance`       | EC2 DescribeInstances        |
| `aws_s3_bucket`      | HeadBucket + GetBucketTagging |
| `aws_security_group` | DescribeSecurityGroups       |
| `aws_vpc`            | DescribeVpcs                 |
| `aws_subnet`         | DescribeSubnets              |
| `aws_iam_role`       | GetRole + ListRoleTags       |

Unknown types are skipped with a warning. Data sources in state are ignored.

## Layout

- `cmd/tdd` — CLI (`scan`, `serve`)
- `internal/state` — Terraform state v3/v4 + local/S3 loaders
- `internal/model` — shared `Resource` / `Drift` / `Report`
- `internal/provider` — registry; `aws` implementation; Azure/GCP stubs
- `internal/compare` — drift engine
- `internal/scan` — orchestrator
- `internal/report` — table + JSON + file store
- `internal/schedule` — cron runner
- `internal/http` + `web/` — thin dashboard
- `testdata/` — sample state fixtures

## Extending providers

Implement `provider.Fetcher`:

```go
type Fetcher interface {
  Name() string
  Supports(tfType string) bool
  Fetch(ctx context.Context, expected []model.Resource) ([]model.Resource, error)
}
```

Register it in `cmd/tdd` next to the AWS fetcher. Map live API fields onto Terraform-like attribute names (`instance_type`, `cidr_block`, `tags`) so the comparer stays provider-agnostic.

Replace `provider.AzureStub()` / `provider.GCPStub()` when those APIs are wired.

## Tests

```bash
go test ./...
```
