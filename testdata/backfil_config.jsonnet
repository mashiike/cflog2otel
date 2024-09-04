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
  backfill: {
    enabled: true,
    time_tolerance: '15m',
  },
  metrics: [
    {
      name: 'http.server.http_requests',
      description: 'The request count of HTTP requests',
      type: 'Count',
      unit: 'ms',
      attributes: [
        {
          key: 'http.status_code',
          value: cel('log.scStatusCategory'),
        },
      ],
    },
  ],
}
