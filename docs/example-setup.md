# Example setup: Postgres end-to-end

Example setup for a Postgres database: service, metadata extraction, profiling, and data quality checks.

## Prerequisites

- The operator is running (see [Getting started](../README.md#getting-started))
- A Kubernetes Secret with your OpenMetadata JWT token
- (Optional) Kubernetes Secrets with your database credentials, if using `valueFrom.secretKeyRef` instead of plain `value:`

## 1. OpenMetadata connection

Shared by all resources below.

```yaml
apiVersion: openmetadata.vortexa.com/v1alpha1
kind: OpenMetadataConnection
metadata:
  name: om-connection
spec:
  url: "http://openmetadata.openmetadata:8585/api"
  authSecretRef:
    name: openmetadata-api-secret
    namespace: openmetadata
    key: token
```

## 2. Service

```yaml
apiVersion: openmetadata.vortexa.com/v1alpha1
kind: OpenMetadataService
metadata:
  name: backend-db
  namespace: default
spec:
  forOpenMetadata:
    serviceType: Postgres
    displayName: "Backend Database"
    connection:
      config:
        type:
          value: Postgres
        hostPort:
          valueFrom:
            secretKeyRef:
              name: my-db-credentials
              key: endpoint
        database:
          value: backend
        username:
          valueFrom:
            secretKeyRef:
              name: my-db-credentials
              key: username
        authType:
          password:
            valueFrom:
              secretKeyRef:
                name: my-db-credentials
                key: password
        sslMode:
          value: prefer
        supportsMetadataExtraction:
          value: true
  openMetadataConnectionRef: om-connection
```

## 3. Metadata ingestion

Extracts tables from the `public` schema every 6 hours.

```yaml
apiVersion: openmetadata.vortexa.com/v1alpha1
kind: IngestionPipeline
metadata:
  name: backend-db-metadata
  namespace: default
spec:
  forOpenMetadata:
    pipelineType: metadata
    service:
      fullyQualifiedName: backend-db
      type: databaseService
    sourceConfig:
      config:
        type: DatabaseMetadata
        markDeletedTables: true
        schemaFilterPattern:
          includes:
            - public
    airflowConfig:
      scheduleInterval: "0 */6 * * *"
  openMetadataConnectionRef: om-connection
```

## 4. Profiler

Computes table and column metrics every 12 hours.

```yaml
apiVersion: openmetadata.vortexa.com/v1alpha1
kind: IngestionPipeline
metadata:
  name: backend-db-profiler
  namespace: default
spec:
  forOpenMetadata:
    pipelineType: profiler
    service:
      fullyQualifiedName: backend-db
      type: databaseService
    sourceConfig:
      config:
        type: Profiler
        profileSample: 100
        computeTableMetrics: true
        computeColumnMetrics: true
    airflowConfig:
      scheduleInterval: "0 */12 * * *"
  openMetadataConnectionRef: om-connection
```

## 5. Test cases

A not-null check (no params), a range check (with params), and a custom SQL query on the `orders` table.

```yaml
apiVersion: openmetadata.vortexa.com/v1alpha1
kind: OpenMetadataTestCase
metadata:
  name: orders-customer-id-not-null
  namespace: default
spec:
  forOpenMetadata:
    testDefinition: columnValuesToBeNotNull
    entityLink: "<#E::table::backend-db.backend.public.orders::columns::customer_id>"
  openMetadataConnectionRef: om-connection
---
apiVersion: openmetadata.vortexa.com/v1alpha1
kind: OpenMetadataTestCase
metadata:
  name: orders-total-amount-range
  namespace: default
spec:
  forOpenMetadata:
    testDefinition: columnValuesToBeBetween
    entityLink: "<#E::table::backend-db.backend.public.orders::columns::total_amount>"
    parameterValues:
      - name: minValue
        value: "0"
      - name: maxValue
        value: "100000"
  openMetadataConnectionRef: om-connection
---
apiVersion: openmetadata.vortexa.com/v1alpha1
kind: OpenMetadataTestCase
metadata:
  name: orders-no-negative-totals
  namespace: default
spec:
  forOpenMetadata:
    testDefinition: tableCustomSQLQuery
    entityLink: "<#E::table::backend-db.backend.public.orders>"
    parameterValues:
      - name: sqlExpression
        value: "SELECT COUNT(*) FROM orders WHERE total_amount < 0"
      - name: strategy
        value: "COUNT"
      - name: threshold
        value: "0"
    displayName: "No Negative Order Totals"
  openMetadataConnectionRef: om-connection
```

## 6. Test suite pipeline

Schedule the test cases to run. Each table you want to test needs its own test suite pipeline.

```yaml
apiVersion: openmetadata.vortexa.com/v1alpha1
kind: IngestionPipeline
metadata:
  name: orders-test-suite
  namespace: default
spec:
  forOpenMetadata:
    pipelineType: TestSuite
    service:
      fullyQualifiedName: backend-db.backend.public.orders.testSuite
      type: testSuite
    sourceConfig:
      config:
        type: TestSuite
        entityFullyQualifiedName: backend-db.backend.public.orders.testSuite
    airflowConfig:
      scheduleInterval: "0 */12 * * *"
  openMetadataConnectionRef: om-connection
```

## Verifying

```bash
kubectl get openmetadataservices,ingestionpipelines,openmetadatatestcases
```

Each resource's `READY` condition shows whether it has been successfully reconciled with OpenMetadata.
