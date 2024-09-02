
resource "aws_iam_role" "cflog2otel" {
  name = "cflog2otel-lambda"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Sid    = ""
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })
}


resource "aws_iam_policy" "cflog2otel" {
  name   = "cflog2otel"
  path   = "/"
  policy = data.aws_iam_policy_document.cflog2otel.json
}

resource "aws_cloudwatch_log_group" "cflog2otel" {
  name              = "/aws/lambda/cflog2otel"
  retention_in_days = 7
}

resource "aws_iam_role_policy_attachment" "cflog2otel" {
  role       = aws_iam_role.cflog2otel.name
  policy_arn = aws_iam_policy.cflog2otel.arn
}

data "aws_iam_policy_document" "cflog2otel" {
  statement {
    actions = [
      "sqs:DeleteMessage",
      "sqs:GetQueueUrl",
      "sqs:ChangeMessageVisibility",
      "sqs:ReceiveMessage",
      "sqs:GetQueueAttributes",
    ]
    resources = [aws_sqs_queue.cflog2otel.arn]
  }
  statement {
    actions = [
      "ssm:GetParameter*",
    ]
    resources = ["*"]
  }
  statement {
    actions = [
      "s3:GetObject",
    ]
    resources = ["*"]
  }
  statement {
    actions = [
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:PutLogEvents",
    ]
    resources = ["*"]
  }
}

resource "aws_sqs_queue" "cflog2otel" {
  name                       = "cflog2otel"
  message_retention_seconds  = 86400
  visibility_timeout_seconds = 600
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.cflog2otel-dlq.arn
    maxReceiveCount     = 3
  })
}

data "aws_iam_policy_document" "cflog2otel_sqs" {
  statement {
    sid    = "S3Notification"
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["s3.amazonaws.com"]
    }

    actions   = ["sqs:SendMessage"]
    resources = [aws_sqs_queue.cflog2otel.arn]

    condition {
      test     = "ArnEquals"
      variable = "aws:SourceArn"
      values   = [aws_s3_bucket.logs.arn]
    }
  }
}

resource "aws_sqs_queue_policy" "test" {
  queue_url = aws_sqs_queue.cflog2otel.id
  policy    = data.aws_iam_policy_document.cflog2otel_sqs.json
}

resource "aws_sqs_queue" "cflog2otel-dlq" {
  name                      = "cflog2otel-dlq"
  message_retention_seconds = 345600
}

data "archive_file" "cflog2otel_dummy" {
  type        = "zip"
  output_path = "${path.module}/cflog2otel_dummy.zip"
  source {
    content  = "cflog2otel_dummy"
    filename = "bootstrap"
  }
  depends_on = [
    null_resource.cflog2otel_dummy
  ]
}

resource "null_resource" "cflog2otel_dummy" {}

resource "aws_lambda_function" "cflog2otel" {
  lifecycle {
    ignore_changes = all
  }

  function_name = "cflog2otel"
  role          = aws_iam_role.cflog2otel.arn
  architectures = ["arm64"]
  handler       = "bootstrap"
  runtime       = "provided.al2023"
  filename      = data.archive_file.cflog2otel_dummy.output_path
}

resource "aws_lambda_alias" "cflog2otel" {
  lifecycle {
    ignore_changes = all
  }
  name             = "current"
  function_name    = aws_lambda_function.cflog2otel.arn
  function_version = aws_lambda_function.cflog2otel.version
}

resource "aws_lambda_event_source_mapping" "cflog2otel_invoke_from_sqs" {
  batch_size       = 1
  event_source_arn = aws_sqs_queue.cflog2otel.arn
  enabled          = true
  function_name    = aws_lambda_alias.cflog2otel.arn
  depends_on = [
    aws_s3_bucket.logs,
    aws_sqs_queue.cflog2otel,
  ]
}

resource "aws_ssm_parameter" "mackerel_apikey" {
  name        = "/cflog2otel/MACKEREL_APIKEY"
  description = "Mackerel API Key for cflog2otel ${local.mackerel_apikey_source}"
  type        = "SecureString"
  value       = local.mackerel_apikey
}


resource "aws_s3_bucket" "logs" {
  bucket = local.s3_bucket_name
}

resource "aws_s3_bucket_notification" "logs_notification" {
  bucket = aws_s3_bucket.logs.id

  queue {
    id            = "cflog2otel"
    queue_arn     = aws_sqs_queue.cflog2otel.arn
    events        = ["s3:ObjectCreated:*"]
    filter_prefix = "cf-logs/"
    filter_suffix = ".gz"
  }
}

data "aws_caller_identity" "current" {}
