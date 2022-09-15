# Stacker
`stacker` is a golang-based AWS Lambda function that will set up a F5 Application Infrastructure Protection (AIP/Threat Stack) AWS integration for EC2 sync when a new account is created in your AWS organization. It can handle AWS EventBridge notifications from the AWS Organizations API (specifically, the `CreateAccountResult` event) **or** the AWS Control Tower API (the `CreateManagedAccount` event).

## Requirements
This integration only works on account creation. Further updating of policies (for example, if F5 AIP introduces new functionality) will need to be handled separately.

`stacker` requires a universally deployed role that it can use to create the IAM role in a brand new AWS account. If you're using the AWS Organizations API, it's common to have a `OrganizationAccountAccessRole` IAM role provisioned automatically with every new account that can be used for this purpose. If you're using Control Tower, `AWSControlTowerExecution` is the role you want to use instead. 

## Building the Lambda

You'll need a golang toolchain installed. After that, simply run `make`. This will build a zip file with the Stacker binary and required JSON files that you'll need for configuration and deployment.

## Configuration and Deployment

### Terraform
Take a look at [deploy.tf](deploy.tf) to deploy all of the following using Terraform. This file is meant to be customized for your environment.

**Note:** If you use our Terraform deployment, you'll still need to create the SecureString parameter manually since we prefer not to store the API key in the Terraform state. Your threat model may differ. Reference the "Parameter Store" step below for instructions on how to do that through the AWS Console.

### Manual Setup

#### Parameter Store
[Create a SecureString parameter](https://console.aws.amazon.com/systems-manager/parameters/create) that has your F5 AIP REST API key in it. You can find this information in the F5 AIP UI - click "Settings" on the left, then click "Application Keys" on the top navigation bar.

#### Lambda IAM Role
Create an IAM role for your Lambda job. It will need the ability to sts:AssumeRole into whichever universally deployed role is available across your account structure (see "Requirements" above), write logs to CloudWatch, and pull a value from SSM Parameter Store.

With the IAM policy below, remember to replace:
* `ACCOUNT_ID` with your organization account ID number
* `API_KEY_ID` with the parameter name of your API key
* `UNIVERSALLY_DEPLOYED_ROLE` with `OrganizationAccountAccessRole` or `AWSControlTowerExecution` (or a custom value, if you've set that up yourself).
* `KEY_ID` for the KMS key ID used to encrypt the SSM SecureString value
```
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AccessAccountAndKeys",
      "Effect": "Allow",
      "Action": [
        "sts:AssumeRole",
        "ssm:GetParameter",
        "kms:Decrypt"
      ],
      "Resource": [
        "arn:aws:iam::*:role/UNIVERSALLY_DEPLOYED_ROLE",
        "arn:aws:ssm:us-east-1:ACCOUNT_ID:parameter/API_KEY_ID",
        "arn:aws:kms:us-east-1:ACCOUNT_ID:key/KEY_ID"
      ]
    },
    {
      "Sid": "CreateLogGroups",
      "Effect": "Allow",
      "Action": "logs:CreateLogGroup",
      "Resource": "arn:aws:logs:us-east-1:ACCOUNT_ID:*"
    },
    {
      "Sid": "WriteLogs",
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogStream",
        "logs:PutLogEvents"
      ],
      "Resource": [
        "arn:aws:logs:us-east-1:ACCOUNT_ID:log-group:/aws/lambda/stacker:*"
      ]
    }
  ]
}
```

#### Lambda Environment
Create a new integration in Lambda, and use these variables to configure the function:

| Variable                         | Required | Description                                                                   |
|----------------------------------|----------|-------------------------------------------------------------------------------|
| F5_API_KEY_PATH                  | Yes      | The name of the SSM Parameter Store variable where the API key is stored.     |
| F5_API_PATH                      | No       | Base API URL. Defaults to `https://api.threatstack.com`.                      |
| F5_EC2_REGIONS                   | Yes      | Comma-separated list of regions you want to use EC2 sync in. |
| F5_TARGET_ACCOUNT_EXECUTION_ROLE | Yes      | The execution role that will be used to add an IAM Role. Commonly `OrganizationAccountAccessRole` or `AWSControlTowerExecution`. |
| F5_TARGET_ROLE_NAME              | Yes      | Name of the role you want to create in the target account.                    |
| F5_ORG_ID                        | Yes      | F5 AIP Organization ID - check Settings => Application Keys in the UI.  |
| F5_USER_ID                       | Yes      | F5 AIP User ID - check Settings => Application Keys in the UI.          |

#### EventBridge
Next, set up an [EventBridge rule](https://console.aws.amazon.com/events/home?region=us-east-1#/rules/create) to notify the `stacker` lambda function when a new account is created. 

We recommend performing the action on account creation. This API call happens for both the Organizations API or Control Tower. The rule pattern you'll want to use for AWS Organizations API notification is: 
```
{
  "detail": {
    "eventName": [
      "CreateAccountResult"
    ],
    "serviceEventDetails": {
      "createAccountStatus": {
        "state": [
          "SUCCEEDED"
        ]
      }
    }
  },
  "detail-type": [
    "AWS Service Event via CloudTrail"
  ],
  "source": [
    "aws.organizations"
  ]
}
```

If you'd rather listen for a notification from the AWS Control Tower service, you can use:
```
{
  "detail": {
    "eventName": [
      "CreateManagedAccount"
    ],
    "serviceEventDetails": {
      "createManagedAccountStatus": {
        "state": [
          "SUCCEEDED"
        ]
      }
    }
  },
  "detail-type": [
    "AWS Service Event via CloudTrail"
  ],
  "source": [
    "aws.controltower"
  ]
}
```

## Monitoring
You can use CloudWatch Alarms to alert on execution failures. This will be a helpful hint in the event of a provision request that doesn't
work. Information on what failed would be in CloudWatch Logs for that execution.

## Debugging
You can replay events into the Lambda function, you'll just need to wrap it in an EventBridge event. Go get the raw JSON for the event from
CloudTrail, and use that object for the `detail` object in a sample EventBridge Event. Sample messages can be found in the `samples/` folder.