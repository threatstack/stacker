# stacker - automatically integrate new AWS accounts in your AWS organization
#           with F5 Application Infrastructure Protection (AIP)
#
# deploy.tf: Sample deployment terraform code
#
# Copyright 2022 F5 Inc.
# Licensed under the Apache License, Version 2.0 (the "License"); see
# the LICENSE file in this repository for more information.
#
#########################################################################################
## Begin Configurable Values
# Name of the universally deployed role that stacker will use to create an IAM
# role in the brand new target account. Typically "OrganizationAccountAccessRole" -
# but if you're using Control Tower, try "AWSControlTowerExecution" instead.
variable "targetAccountExecutionRole" {
  type  = string
  default = "OrganizationAccountAccessRole"
}

# Name of the SSM SecureString parameter you created with your F5 AIP API key.
# We don't recommend defining this here, since it'd store the value in your state file,
# so we use a data resource
variable "F5APIKeySSMName" {
  type  = string
  default = "F5AIPKey"
}

# Name of the KMS key used to encrypt the secret. You'll need to update the key policy on your own
variable "F5APIKeyKMSKey" {
  type = string
  default = "alias/aws/ssm"
}

# Name of the Lambda function we'll make
variable "lambdaName" {
  type  = string
  default = "f5-aip-stacker"
}

# F5 AIP Org ID
variable "F5AIPOrgID" {
  type  = string
  default = ""
}

# F5 AIP User ID (for the API key)
variable "F5AIPUserID" {
  type  = string
  default = ""
}

# EC2 Sync Regions to enable, comma separated
variable "ec2SyncRegions" {
  type  = string
  default = "us-east-1"
}

# Name of the role you want to create in the new account
variable "targetRoleName" {
  type  = string
  default = "f5aip-integration"
}

# What event notification to use to set up the integration. Two options:
# Organizations API:
# {"detail": {"eventName":["CreateAccountResult"],"serviceEventDetails":{"createAccountStatus":{"state":["SUCCEEDED"]}}},"detail-type":["AWS Service Event via CloudTrail"],"source":["aws.organizations"]}
#
# or Control Tower API:
# {"detail": {"eventName":["CreateManagedAccount"],"serviceEventDetails":{"createManagedAccountStatus":{"state":["SUCCEEDED"]}}},"detail-type":["AWS Service Event via CloudTrail"],"source":["aws.controltower"]}
variable "eventBridgePattern" {
  type  = string
  default = <<EOF
{"detail": {"eventName":["CreateAccountResult"],"serviceEventDetails":{"createAccountStatus":{"state":["SUCCEEDED"]}}},"detail-type":["AWS Service Event via CloudTrail"],"source":["aws.organizations"]}
EOF
}
## End Configurable Values
#########################################################################################
# Information on the account this is running in for use later
data "aws_caller_identity" "current" {}

# We use the built-in SSM account ID since it's in the organization account, and
# there are likely few who access it. If you need to keep it more secure, create
# a separate KMS key instead.
#
# If you choose to make a separate KMS key, make sure you allow the kms:Decrypt
# permission in the resource policy for the F5AIPStackerLambda role.
data "aws_kms_key" "ssm" {
  key_id = var.F5APIKeyKMSKey
}

# Create the execution policy for the AIP lambda.
data "aws_iam_policy_document" "lambdaARP" {
  statement {
    actions = ["sts:AssumeRole"]
    effect  = "Allow"
    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "F5AIPStackerLambda" {
  name               = "F5AIPStackerLambda"
  description        = "Execution role for F5 AIP Integration on Account Creation"
  assume_role_policy = data.aws_iam_policy_document.lambdaARP.json
}

data "aws_iam_policy_document" "F5AIPIntegraton" {
  statement {
    sid    = "AccessAccountAndKeys"
    effect = "Allow"
    actions = [
      "sts:AssumeRole",
      "ssm:GetParameter",
      "kms:Decrypt"
    ]
    resources = [
      "arn:aws:iam::*:role/${var.targetAccountExecutionRole}",
      data.aws_kms_key.ssm.arn,
      "arn:aws:ssm:us-east-1:${data.aws_caller_identity.current.account_id}:parameter/${var.F5APIKeySSMName}"
    ]
  }
  statement {
    sid       = "CreateLogGroups"
    effect    = "Allow"
    actions   = ["logs:CreateLogGroup"]
    resources = ["arn:aws:logs:us-east-1:${data.aws_caller_identity.current.account_id}:*"]
  }
  statement {
    sid    = "WriteF5StackerLogs"
    effect = "Allow"
    actions = [
      "logs:CreateLogStream",
      "logs:PutLogEvents"
    ]
    resources = [
      "arn:aws:logs:us-east-1:${data.aws_caller_identity.current.account_id}:log-group:/aws/lambda/${var.lambdaName}:*"
    ]
  }
}

resource "aws_iam_role_policy" "F5AIPIntegration" {
  name   = "f5-aip-integrator-access"
  role   = aws_iam_role.F5AIPStackerLambda.id
  policy = data.aws_iam_policy_document.F5AIPIntegraton.json
}

# Create the Lambda
resource "aws_lambda_function" "F5AIPStacker" {
  function_name = var.lambdaName
  role          = aws_iam_role.F5AIPStackerLambda.arn
  handler       = "stacker"
  runtime       = "go1.x"
  memory_size   = "128"
  filename      = "stacker.zip"
  environment {
    variables = {
      F5_API_KEY_PATH                  = var.F5APIKeySSMName,
      F5_EC2_REGIONS                   = var.ec2SyncRegions
      F5_TARGET_ACCOUNT_EXECUTION_ROLE = var.targetAccountExecutionRole
      F5_TARGET_ROLE_NAME              = var.targetRoleName
      F5_ORG_ID                        = var.F5AIPOrgID
      F5_USER_ID                       = var.F5AIPUserID
    }
  }
}

# Glue it together with EventBridge
resource "aws_cloudwatch_event_rule" "F5AIPInvoker" {
  name          = "F5AIPInvoker"
  description   = "Rule to send new organization creation events to F5 AIP Stacker"
  event_pattern = var.eventBridgePattern
}

resource "aws_cloudwatch_event_target" "F5AIPInvoker" {
  rule      = aws_cloudwatch_event_rule.F5AIPInvoker.name
  target_id = "SendToAIPInvoker"
  arn       = aws_lambda_function.F5AIPStacker.arn
}

resource "aws_lambda_permission" "allow_eventBridge" {
  statement_id  = "AllowExecutionFromEventBridge"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.F5AIPStacker.arn
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.F5AIPInvoker.arn
}
