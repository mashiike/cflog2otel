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
      name: cel('"http.server.index_requests"'),  // name can not use CEL
      description: 'The number of HTTP requests for index.html',
      type: 'Count',
      filter: cel('log.csUriStem == "/index.html"'),
    },
  ],
}
