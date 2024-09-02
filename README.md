# cflog2otel

`cflog2otel` is a Lambda Function designed to aggregate AWS CloudFront access logs and export the metrics using the OpenTelemetry (Otel) protocol. When CloudFront logs are uploaded to S3, the Lambda is triggered by an S3 Notification, and it automatically collects and monitors important metrics in real-time.

## Features

- **CloudFront Log S3 Event Handling**
  - Automatically processes CloudFront logs stored in S3 whenever an S3 Notification Event is triggered.
- **Operates as a Lambda Function**
  - Runs on AWS Lambda, allowing for easy serverless scaling.
- **OpenTelemetry Compatible**
  - Aggregated metrics are exported using the OpenTelemetry protocol, compatible with various monitoring tools and platforms.
- **Flexible Configuration**
  - Customize the log fields to be aggregated, such as request status codes or byte counts.

## Installation

Pre-built release binaries are provided. You can deploy the binary as a Lambda function to aggregate CloudFront logs in real-time. This binary acts as the `bootstrap` for the Lambda.

### 1. Download the Release Binary

Download the latest binary from the [Releases page](https://github.com/mashiike/cflog2otel/releases).

### 2. Create the Lambda Function

1. **Create the Lambda Function**:
   - Use the AWS Lambda console to create a new Lambda function and select a custom runtime.
   - Upload the downloaded binary as the `bootstrap` for the Lambda function.

2. **Set Up S3 Notification Trigger**:
   - Configure an S3 Notification on the S3 bucket where CloudFront logs are stored to trigger the Lambda function.


### Workflow

1. **S3 Notification Event**: When CloudFront logs are uploaded to S3, an S3 Notification Event triggers the Lambda function.
2. **Lambda Function Execution**: The `cflog2otel` Lambda function is invoked by the event.
3. **Log Download and Aggregation**: The Lambda function downloads the log file from S3 and aggregates the metrics based on the specified configuration.
4. **Metrics Export**: The aggregated metrics are exported to the specified OpenTelemetry endpoint.

## Configuration

Below is an example of the configuration file used to parse CloudFront logs and export metrics when the Lambda function is triggered by an S3 event.

```jsonnet
local cel = std.native('cel');

{
  otel: {
    endpoint: 'http://localhost:4317/',
    gzip: true,
  },
  resource_attributes: [
    {
      key: 'service.name',
      value: 'Amazon CloudFront',
    },
    {
      key: 'aws.cloudfront.distribution_id',
      value: cel('cloudfront.distributionId'),
    },
  ],
  scope: {
    name: 'cflog2otel',
    version: '0.0.0',
    schema_url: 'https://example.com/schemas/1.0.0',
  },
  metrics: [
    {
      name: 'http.server.requests',
      description: 'The number of HTTP requests',
      type: 'Count',
      attributes: [
        {
          key: 'http.status_code',
          value: cel('log.scStatusCategory'),
        },
      ],
    },
  ],
}
```

#### Example of Metrics Aggregation for CloudFront Logs

Consider a CloudFront log file, such as the one described in the [AWS CloudFront Developer Guide](https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/AccessLogs.html), with the following entries:

**Log File Example**  
Path: `example-prefix/EMLARXS9EXAMPLE.2019-11-14-20.RT4KCN4SGK9.gz`
```plaintext
#Version: 1.0
#Fields: date time x-edge-location sc-bytes c-ip cs-method cs(Host) cs-uri-stem sc-status cs(Referer) cs(User-Agent) cs-uri-query cs(Cookie) x-edge-result-type x-edge-request-id x-host-header cs-protocol cs-bytes time-taken x-forwarded-for ssl-protocol ssl-cipher x-edge-response-result-type cs-protocol-version fle-status fle-encrypted-fields c-port time-to-first-byte x-edge-detailed-result-type sc-content-type sc-content-len sc-range-start sc-range-end
2019-12-04	21:02:31	LAX1	392	192.0.2.100	GET	d111111abcdef8.cloudfront.net	/index.html	200	-	Mozilla/5.0%20(Windows%20NT%2010.0;%20Win64;%20x64)%20AppleWebKit/537.36%20(KHTML,%20like%20Gecko)%20Chrome/78.0.3904.108%20Safari/537.36	-	-	Hit	SOX4xwn4XV6Q4rgb7XiVGOHms_BGlTAC4KyHmureZmBNrjGdRLiNIQ==	d111111abcdef8.cloudfront.net	https	23	0.001	-	TLSv1.2	ECDHE-RSA-AES128-GCM-SHA256	Hit	HTTP/2.0	-	-	11040	0.001	Hit	text/html	78	-	-
2019-12-04	21:02:31	LAX1	392	192.0.2.100	GET	d111111abcdef8.cloudfront.net	/index.html	200	-	Mozilla/5.0%20(Windows%20NT%2010.0;%20Win64;%20x64)%20AppleWebKit/537.36%20(KHTML,%20like%20Gecko)%20Chrome/78.0.3904.108%20Safari/537.36	-	-	Hit	k6WGMNkEzR5BEM_SaF47gjtX9zBDO2m349OY2an0QPEaUum1ZOLrow==	d111111abcdef8.cloudfront.net	https	23	0.000	-	TLSv1.2	ECDHE-RSA-AES128-GCM-SHA256	Hit	HTTP/2.0	-	-	11040	0.000	Hit	text/html	78	-	-
2019-12-04	21:02:31	LAX1	392	192.0.2.100	GET	d111111abcdef8.cloudfront.net	/index.html	200	-	Mozilla/5.0%20(Windows%20NT%2010.0;%20Win64;%20x64)%20AppleWebKit/537.36%20(KHTML,%20like%20Gecko)%20Chrome/78.0.3904.108%20Safari/537.36	-	-	Hit	f37nTMVvnKvV2ZSvEsivup_c2kZ7VXzYdjC-GUQZ5qNs-89BlWazbw==	d111111abcdef8.cloudfront.net	https	23	0.001	-	TLSv1.2	ECDHE-RSA-AES128-GCM-SHA256	Hit	HTTP/2.0	-	-	11040	0.001	Hit	text/html	78	-	-
2019-12-13	22:36:27	SEA19-C1	900	192.0.2.200	GET	d111111abcdef8.cloudfront.net	/favicon.ico	502	http://www.example.com/	Mozilla/5.0%20(Windows%20NT%2010.0;%20Win64;%20x64)%20AppleWebKit/537.36%20(KHTML,%20like%20Gecko)%20Chrome/78.0.3904.108%20Safari/537.36	-	-	Error	1pkpNfBQ39sYMnjjUQjmH2w1wdJnbHYTbag21o_3OfcQgPzdL2RSSQ==	www.example.com	http	675	0.102	-	-	-	Error	HTTP/1.1	-	-	25260	0.102	OriginDnsError	text/html	507	-	-
2019-12-13	22:36:26	SEA19-C1	900	192.0.2.200	GET	d111111abcdef8.cloudfront.net	/	502	-	Mozilla/5.0%20(Windows%20NT%2010.0;%20Win64;%20x64)%20AppleWebKit/537.36%20(KHTML,%20like%20Gecko)%20Chrome/78.0.3904.108%20Safari/537.36	-	-	Error	3AqrZGCnF_g0-5KOvfA7c9XLcf4YGvMFSeFdIetR1N_2y8jSis8Zxg==	www.example.com	http	735	0.107	-	-	-	Error	HTTP/1.1	-	-	3802	0.107	OriginDnsError	text/html	507	-	-
2019-12-13	22:37:02	SEA19-C2	900	192.0.2.200	GET	d111111abcdef8.cloudfront.net	/	502	-	curl/7.55.1	-	-	Error	kBkDzGnceVtWHqSCqBUqtA_cEs2T3tFUBbnBNkB9El_uVRhHgcZfcw==	www.example.com	http	387	0.103	-	-	-	Error	HTTP/1.1	-	-	12644	0.103	OriginDnsError	text/html	507	-	-
```

For the above log file, the following metrics aggregation result will be exported:

**Metrics Aggregation Result Example**  

```json
{
  "resource_metrics": [
    {
      "resource": {
        "attributes": [
          {
            "key": "aws.cloudfront.distribution_id",
            "value": {
              "Value": {
                "StringValue": "EMLARXS9EXAMPLE"
              }
            }
          },
          {
            "key": "service.name",
            "value": {
              "Value": {
                "StringValue": "Amazon CloudFront"
              }
            }
          }
        ]
      },
      "scope_metrics": [
        {
          "scope": {
            "name": "cflog2otel",
            "version": "0.0.0"
          },
          "metrics": [
            {
              "name": "http.server.requests",
              "description": "The number of HTTP requests",
              "Data": {
                "Sum": {
                  "data_points": [
                    {
                      "attributes": [
                        {
                          "key": "http.status_code",
                          "value": {
                            "Value": {
                              "StringValue": "2xx"
                            }
                          }
                        }
                      ],
                      "start_time_unix_nano": 1575493320000000000,
                      "Value": {
                        "AsInt": 3
                      }
                    },
                    {
                      "attributes": [
                        {
                          "key": "http.status_code",
                          "value": {
                            "Value": {
                              "StringValue": "5xx"
                            }
                          }
                        }
                      ],
                      "start_time_unix_nano": 1576276560000000000,
                      "Value": {
                        "AsInt": 2
                      }
                    },
                    {
                      "attributes": [
                        {
                          "key": "http.status_code",
                          "value": {
                            "Value": {
                              "StringValue": "5xx"
                            }
                          }
                        }
                      ],
                      "start_time_unix_nano": 1576276620000000000,
                      "Value": {
                        "AsInt": 1
                      }
                    }
                  ],
                  "aggregation_temporality": 2,
                  "is_monotonic": true
                }
              }
            }
          ],
          "schema_url": "https://example.com/schemas/1.0.0"
        }
      ]
    }
  ]
}
```

In this example, the HTTP requests are aggregated by their status code categories (e.g., 2xx, 5xx) and the results are exported as metrics.

### OpenTelemetry Provider Export Configuration

The `otel` field is used to specify settings related to exporting metrics to an OpenTelemetry provider.

#### Field Descriptions

- endpoint (string, optional):
  - Specifies the endpoint URL where the metrics data should be sent.
- gzip (bool, optional):
  - Indicates whether to enable GZip compression when exporting metrics data.
- eaders (map[string]string, optional):
  - A map of HTTP headers used when sending metrics data. This can include headers such as Authorization.

#### Example Using ssm

Config can use https://github.com/fujiwara/ssm-lookup to get the value from AWS SSM Parameter Store.  
Here is an example that demonstrates how to use AWS Systems Manager (SSM) Parameter Store to securely retrieve an API key and set it as the Authorization header.

```jsonnet
local ssm = std.native('ssm');

{
  otel: {
    endpoint: 'https://otel-collector.example.com/',
    gzip: true,
    headers: {
      "Authorization": 'Bearer '+ssm("/path/to/api-key")
    },
  },
}
```

### OpenTelemetry Metrics Aggregation Settings

The `resource_attributes`, `scope`, and `metrics` fields are used to configure how metrics are aggregated and exported to an OpenTelemetry provider.

#### `cel` and `cel_switch` Jsonnet Native Functions

The `cel` and `cel_switch` Jsonnet native functions allow you to define custom expressions for filtering and calculating metrics. These functions use [CEL (Common Expression Language)](https://cel.dev) to define logic based on the log data, enabling precise control over which metrics are collected and how they are calculated.

For example, `cel('log.scStatusCategory == "5xx"')` is a custom expression that filters logs based on the HTTP status code category.

The `cel_switch` function is a syntax sugar for a switch-case statement in CEL. An example is shown below:

```jsonnet
cel_switch([
    {
        case: 'log.scStatusCategory == "2xx"',
        value: 'success',
    },
    {
        case: 'log.scStatusCategory == "4xx"',
        value: 'client_error',
    },
    {
        case: 'log.scStatusCategory == "5xx"',
        value: 'server_error',
    },
    {
        default: 'other',
    },
])
```

#### CEL Variables

CEL variables have the following object structure:

| Variable Name   | Type     | Description                                                                        |
|-----------------|----------|------------------------------------------------------------------------------------|
| `bucket`        | `object` | Contains information about the S3 bucket provided in the Event Notification.       |
| `object`        | `object` | Contains information about the S3 object provided in the Event Notification.       |
| `cloudfront`    | `object` | Contains information about the CloudFront distribution related to the log.         |
| `log`           | `object` | Contains a single line of CloudFront log information. Each field in the `log` object is a lowerCamelCase version of the corresponding CloudFront log field name. |

Below are the specific variables that are commonly used in CEL expressions:

| Variable Name                  | Type               | Description                                                                 |
|--------------------------------|--------------------|-----------------------------------------------------------------------------|
| `bucket.name`                  | `string`           | The name of the S3 bucket where the event occurred.                         |
| `object.key`                   | `string`           | The key (path) of the object within the S3 bucket.                          |
| `cloudfront.distributionId`    | `string`           | The ID of the CloudFront distribution.                                      |
| `log.xEdgeLocation`            | `nullable string`  | The AWS Edge location that served the request.                              |
| `log.csMethod`                 | `nullable string`  | The HTTP method used in the request (e.g., GET, POST).                      |
| `log.csHost`                   | `nullable string`  | The hostname from which the request originated.                             |
| `log.csUriStem`                | `nullable string`  | The URI stem of the request (the path to the resource).                     |
| `log.scStatus`                 | `nullable int`     | The HTTP status code returned by CloudFront (e.g., 200, 404).               |
| `log.scStatusCategory`         | `nullable string`  | The category of the HTTP status code (e.g., `2xx`, `4xx`, `5xx`).           |

##### Usage of `cel` and `cel_switch`

The `cel` and `cel_switch` functions are used in the following configuration fields:

- `resource_attributes[*].value`
- `metrics[*].attributes[*].value`
- `metrics[*].filter`
- `metrics[*].value`

#### `metrics[*].type` is Aggregation type

aggregation type is one of the following:
- `Count` (default): Count the number of log lines that match the filter.
- `Sum`: Sum the value of the specified field in the log lines that match the filter.
- `Histogram`: Calculate the histogram of the specified field in the log lines that match the filter.

##### Example of `Count` Aggregation

Count rows that match the filter.

```jsonnet
local cel = std.native('cel');

{
  otel: {
    endpoint: 'http://localhost:4317/',
    gzip: true,
  },
  metrics: [
    {
      name: 'http.server.requests',
      description: 'The number of HTTP requests',
      type: 'Count',
      attributes: [
        {
          key: 'http.status_code',
          value: cel('log.scStatusCategory'),
        },
      ],
    },
  ],
}
```

required fields are `name`and `type`.
optional fields are `description`, `attributes`, `filter` and `unit`.
Metric Temporality defaults `Delta`, if `is_cumulative` is true, it will be `Cumulative`.

##### Example of `Sum` Aggregation

Sum values of the specified field in the log lines that match the filter.

```jsonnet
local cel = std.native('cel');

{
  otel: {
    endpoint: 'http://localhost:4317/',
    gzip: true,
  },
  metrics: [
    {
      name: 'http.server.total_bytes',
      description: 'The total number of bytes sent by the server',
      type: 'Sum',
      unit: 'Byte',
      attributes: [
        {
          key: 'http.status_code',
          value: cel('log.scStatusCategory'),
        },
      ],
      value: cel('double(log.scBytes)'),
      is_monotonic: true,
    },
  ],
}
```

required fields are `name`, `type` and `value`.
optional fields are `description`, `attributes`, `filter`, `unit` and `is_monotonic`.
Metric Temporality defaults `Delta`, if `is_cumulative` is true, it will be `Cumulative`.
Metric default `Non-Monotonic`, if `is_monotonic` is true, it will be `Monotonic`.

##### Example of `Histogram` Aggregation

Calculate the histogram of the specified field in the log lines that match the filter.

```jsonnet
local cel = std.native('cel');

{
  otel: {
    endpoint: 'http://localhost:4317/',
    gzip: true,
  },
  metrics: [
    {
      name: 'http.server.request_time',
      description: 'The request time of HTTP requests',
      type: 'Histogram',
      unit: 'ms',
      value: cel('log.timeTaken * 1000.0'),
    },
  ],
}
```

required fields are `name`, `type` and `value`.
optional fields are `description`, `attributes`, `filter`, `unit`, `boundaries`, and `no_min_max`.
Metric Temporality defaults `Delta`, if `is_cumulative` is true, it will be `Cumulative`.
Boundaries defaults `[0, 10, 25, 50, 75, 100, 250, 500, 750, 1000, 2500, 5000, 7500, 10000]`, if `boundaries` is specified, it will be used.
If set boundaries `[0.0, 0.5, 1.0, 2.5, 5.0]` means histogram buckets are `(-inf, 0.0], (0.0, 0.5], (0.5, 1.0], (1.0, 2.5], (2.5, 5.0], (5.0, +inf)`.
If `no_min_max` is true, the not  calculate the histogram of the minimum and maximum values.

### Example of Mackerel Labeled Metrics

see [lambda/mackerel](./lambda/mackerel) dir for more details.

include terraform code and [lambroll](https://github.com/fujiwara/lambroll) configuration.

## License

This project is licensed under the MIT License. 
See the [LICENSE](./LICENSE) file for more details.
