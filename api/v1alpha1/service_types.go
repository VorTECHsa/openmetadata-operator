/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

// ServiceType identifies an OpenMetadata service type.
// The operator uses this value to derive the correct API endpoint and to
// populate the serviceType field in the OpenMetadata request payload.
// +kubebuilder:validation:Enum=Postgres;Athena;BigQuery;BigTable;Mysql;Redshift;Snowflake;Timescale;Mssql;Oracle;Hive;Impala;Presto;Trino;Vertica;Glue;MariaDB;Druid;Db2;Clickhouse;Databricks;AzureSQL;DynamoDB;SingleStore;SQLite;DeltaLake;Salesforce;PinotDB;Datalake;DomoDatabase;QueryLog;CustomDatabase;Dbt;SapHana;MongoDB;Cassandra;Couchbase;Greenplum;Doris;StarRocks;UnityCatalog;SAS;Iceberg;Teradata;SapErp;Synapse;Exasol;Cockroach;SSAS;Epic;ServiceNow;Dremio;MicrosoftFabric;Kafka;Redpanda;Kinesis;CustomMessaging;S3;ADLS;GCS;CustomStorage;ElasticSearch;OpenSearch;CustomSearch
type ServiceType string

// Database service types.
const (
	ServiceTypePostgres        ServiceType = "Postgres"
	ServiceTypeAthena          ServiceType = "Athena"
	ServiceTypeBigQuery        ServiceType = "BigQuery"
	ServiceTypeBigTable        ServiceType = "BigTable"
	ServiceTypeMysql           ServiceType = "Mysql"
	ServiceTypeRedshift        ServiceType = "Redshift"
	ServiceTypeSnowflake       ServiceType = "Snowflake"
	ServiceTypeTimescale       ServiceType = "Timescale"
	ServiceTypeMssql           ServiceType = "Mssql"
	ServiceTypeOracle          ServiceType = "Oracle"
	ServiceTypeHive            ServiceType = "Hive"
	ServiceTypeImpala          ServiceType = "Impala"
	ServiceTypePresto          ServiceType = "Presto"
	ServiceTypeTrino           ServiceType = "Trino"
	ServiceTypeVertica         ServiceType = "Vertica"
	ServiceTypeGlue            ServiceType = "Glue"
	ServiceTypeMariaDB         ServiceType = "MariaDB"
	ServiceTypeDruid           ServiceType = "Druid"
	ServiceTypeDb2             ServiceType = "Db2"
	ServiceTypeClickhouse      ServiceType = "Clickhouse"
	ServiceTypeDatabricks      ServiceType = "Databricks"
	ServiceTypeAzureSQL        ServiceType = "AzureSQL"
	ServiceTypeDynamoDB        ServiceType = "DynamoDB"
	ServiceTypeSingleStore     ServiceType = "SingleStore"
	ServiceTypeSQLite          ServiceType = "SQLite"
	ServiceTypeDeltaLake       ServiceType = "DeltaLake"
	ServiceTypeSalesforce      ServiceType = "Salesforce"
	ServiceTypePinotDB         ServiceType = "PinotDB"
	ServiceTypeDatalake        ServiceType = "Datalake"
	ServiceTypeDomoDatabase    ServiceType = "DomoDatabase"
	ServiceTypeQueryLog        ServiceType = "QueryLog"
	ServiceTypeCustomDatabase  ServiceType = "CustomDatabase"
	ServiceTypeDbt             ServiceType = "Dbt"
	ServiceTypeSapHana         ServiceType = "SapHana"
	ServiceTypeMongoDB         ServiceType = "MongoDB"
	ServiceTypeCassandra       ServiceType = "Cassandra"
	ServiceTypeCouchbase       ServiceType = "Couchbase"
	ServiceTypeGreenplum       ServiceType = "Greenplum"
	ServiceTypeDoris           ServiceType = "Doris"
	ServiceTypeStarRocks       ServiceType = "StarRocks"
	ServiceTypeUnityCatalog    ServiceType = "UnityCatalog"
	ServiceTypeSAS             ServiceType = "SAS"
	ServiceTypeIceberg         ServiceType = "Iceberg"
	ServiceTypeTeradata        ServiceType = "Teradata"
	ServiceTypeSapErp          ServiceType = "SapErp"
	ServiceTypeSynapse         ServiceType = "Synapse"
	ServiceTypeExasol          ServiceType = "Exasol"
	ServiceTypeCockroach       ServiceType = "Cockroach"
	ServiceTypeSSAS            ServiceType = "SSAS"
	ServiceTypeEpic            ServiceType = "Epic"
	ServiceTypeServiceNow      ServiceType = "ServiceNow"
	ServiceTypeDremio          ServiceType = "Dremio"
	ServiceTypeMicrosoftFabric ServiceType = "MicrosoftFabric"
)

// Messaging service types.
const (
	ServiceTypeKafka           ServiceType = "Kafka"
	ServiceTypeRedpanda        ServiceType = "Redpanda"
	ServiceTypeKinesis         ServiceType = "Kinesis"
	ServiceTypeCustomMessaging ServiceType = "CustomMessaging"
)

// Storage service types.
const (
	ServiceTypeS3            ServiceType = "S3"
	ServiceTypeADLS          ServiceType = "ADLS"
	ServiceTypeGCS           ServiceType = "GCS"
	ServiceTypeCustomStorage ServiceType = "CustomStorage"
)

// Search service types.
const (
	ServiceTypeElasticSearch ServiceType = "ElasticSearch"
	ServiceTypeOpenSearch    ServiceType = "OpenSearch"
	ServiceTypeCustomSearch  ServiceType = "CustomSearch"
)
