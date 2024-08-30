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
    name: 'test',
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
