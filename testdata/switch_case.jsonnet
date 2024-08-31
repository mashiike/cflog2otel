local cel = std.native('cel');
local cel_switch = std.native('cel_switch');

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
      attributes: [
        {
          key: 'cloudfront.origin',
          value: cel_switch([
            {
              case: 'log.csUriStem.startsWith("/index.html")',
              value: 'S3',
            },
            {
              case: 'log.csUriStem == "/favicon.ico"',
              value: 'S3',
            },
            {
              default: 'app',
            },
          ]),
        },
        {
          key: 'http.status_code',
          value: cel('log.scStatusCategory'),
        },
      ],
    },
  ],
}
