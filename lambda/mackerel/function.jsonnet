{
  Description: 'github.com/mashiike/cflog2otel',
  Architectures: ['arm64'],
  Environment: {
    Variables: {},
  },
  FunctionName: 'cflog2otel',
  Handler: 'bootstrap',
  MemorySize: 128,
  Role: 'arn:aws:iam::{{ must_env `AWS_ACCOUNT_ID` }}:role/cflog2otel-lambda',
  Runtime: 'provided.al2023',
  Timeout: 600,
  TracingConfig: {
    Mode: 'PassThrough',
  },
}
