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
      name: 'http.server.index_requests',
      description: 'The number of HTTP requests for index.html',
      type: 'Count',
      filter: cel('log.csURIStem == "/index.html"'), // csURIStem is not a valid variables, csUriStem is the correct one
    },
  ],
}
