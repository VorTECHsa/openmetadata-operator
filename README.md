# OpenMetadata Operator

Kubernetes operator for managing [OpenMetadata](https://open-metadata.org/) resources. It talks to the OpenMetadata REST API to reconcile services, ingestion pipelines, and data quality test cases defined as CRs in your cluster. Built with [Kubebuilder](https://book.kubebuilder.io/).

## Features

- **Declarative management** of OpenMetadata services, ingestion pipelines, and test cases as Kubernetes custom resources
- **Observe-compare-converge** reconciliation loop with drift detection
- **Automatic cleanup** via finalizers when resources are deleted
- **API-aligned CRD types**: the `forOpenMetadata` spec closely follows the OpenMetadata REST API payloads, so if you know the API you know the CRDs
- **Opaque connection config**: connector configuration is passed through to OpenMetadata as-is, so the operator doesn't need to understand connector-specific fields
- **Kubernetes-native secret resolution**: connection credentials (passwords, tokens, endpoints) can be referenced from Kubernetes Secrets via `valueFrom.secretKeyRef` and resolved at reconciliation time
- **Status conditions** following Kubernetes conventions (`Ready`)
- **Idempotent**: all mutations use OpenMetadata PUT (upsert) endpoints

## Custom Resource Definitions

The operator currently covers the following resources. Additional CRDs are planned to cover more of the OpenMetadata API surface. Contributions are welcome.

| CRD | Description | OpenMetadata API |
|-----|-------------|------------------|
| `OpenMetadataConnection` | Cluster-scoped connection details for an OpenMetadata server | - |
| `OpenMetadataService` | Database, messaging, storage, or search service registration | `PUT /api/v1/services/{serviceCategory}` |
| `IngestionPipeline` | Metadata, profiler, test suite, usage, or lineage pipelines | Upsert (`PUT`) + deploy (`POST /deploy/{id}`) |
| `OpenMetadataTestCase` | Data quality test case assertions on tables or columns | `PUT /api/v1/dataQuality/testCases` |

All CRDs belong to the `openmetadata.vortexa.com` API group.

### Supported service types

- **Database**: Postgres, Snowflake, BigQuery, Redshift, Databricks, Clickhouse, and 50+ more
- **Messaging**: Kafka, Redpanda, Kinesis
- **Storage**: S3, ADLS, GCS
- **Search**: ElasticSearch, OpenSearch

## Prerequisites

- Kubernetes 1.28+
- An OpenMetadata instance with API access
- An OpenMetadata JWT token stored as a Kubernetes Secret

## Getting started

Install the CRDs:

```bash
kubectl apply -k https://github.com/VorTECHsa/openmetadata-operator/config/crd
```

Deploy the operator to the cluster:

```bash
make deploy IMG=ghcr.io/vortechsa/openmetadata-operator:<tag>
```

Or run it locally during development (requires cluster-admin access):

```bash
make install  # install CRDs
make run      # run the controller against your current kubeconfig
```

## CRD usage

See [docs/example-setup.md](docs/example-setup.md) for a full end-to-end walkthrough (service, pipelines, test cases wired together). Sample manifests for each CRD are in [`config/samples/`](config/samples/).

## Development

```bash
make manifests        # Regenerate CRDs and RBAC from markers
make generate         # Regenerate DeepCopy methods
make build            # Build the manager binary
make test             # Run unit and integration tests (uses envtest)
make lint             # Run golangci-lint
make fmt              # Format code
make vet              # Run go vet
```

## How it works

The operator follows a standard **observe-compare-converge** pattern for each resource:

1. **Observe**: read the desired state from the CR
2. **Compare**: fetch the corresponding entity from OpenMetadata
3. **Converge**: create or update via idempotent PUT; for pipelines, also deploy via POST
4. **Finalise**: on deletion, call OpenMetadata DELETE then remove the finalizer

Resources are re-reconciled every 5 minutes. Errors are retried using controller-runtime's default backoff.

## Licence

[Apache License 2.0](LICENSE)
