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
        {
          key: 'http.host',
          value: cel('log.xHostHeader'),
        },
      ],
      filter: cel('!log.xHostHeader.contains("cloudfront.net")'),
    },
  ],
}
