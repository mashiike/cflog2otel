local cel = std.native('cel');
local ssm = std.native('ssm');

{
  otel: {
    endpoint: 'https://otlp.mackerelio.com:4317/',
    headers: {
      'Mackerel-Api-Key': ssm('/cflog2otel/MACKEREL_APIKEY'),
    },
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
    {
      name: 'http.server.request_time',
      description: 'The request time of HTTP requests',
      type: 'Histogram',
      unit: 'ms',
      value: cel('log.timeTaken * 1000.0'),
    },
  ],
}
