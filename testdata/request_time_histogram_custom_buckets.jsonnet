local cel = std.native('cel');
local generate_interval_array(start, end, step) =
  std.map(
    function(x) x * step + start,
    std.range(0, (end - start) / step)
  );

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
      name: 'http.server.request_time',
      description: 'The request time of HTTP requests',
      type: 'Histogram',
      interval: '5m',
      unit: 'ms',
      value: cel('log.timeTaken * 1000.0'),
      boundaries: generate_interval_array(0, 10, 2),
      is_cumulative: true,
    },
  ],
}
