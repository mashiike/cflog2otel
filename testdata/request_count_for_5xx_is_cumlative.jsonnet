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
      name: 'http.server.5xx_requests',
      description: 'The number of HTTP requests with status code 5xx',
      type: 'Count',
      filter: cel('log.scStatusCategory == "5xx"'),
      is_cumulative: true,
    },
  ],
}
