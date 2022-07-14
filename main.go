// stacker - automatically integrate new AWS accounts in your AWS organization
//           with F5 Application Infrastructure Protection (AIP)
//
// main.go: the AWS Lambda function
//
// Copyright 2022 F5 Inc.
// Licensed under the Apache License, Version 2.0 (the "License"); see
// the LICENSE file in this repository for more information.

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	f5aip "github.com/threatstack/ts/api"
)

func main() {
	lambda.Start(HandleLambdaEvent)
}

func HandleLambdaEvent(event eventBridgeEvent) error {
	// Bulid out our config from our env vars and other information
	config, err := buildConfig()
	if err != nil {
		return fmt.Errorf("unable to bulid config: %s", err)
	}

	// The event from the Orgs API vs the Control Tower API is a bit different.
	// We support both.
	config.TargetAccountID, err = determineAccountID(event)
	if err != nil {
		return fmt.Errorf("unable to get target account id: %s", err.Error())
	}

	// Configure the F5 AIP integration. We need the External ID to actually configure
	// the IAM role in the target account.
	externalID, err := setupF5Integration(config)
	if err != nil {
		return fmt.Errorf("unable to setup integration: %s", err.Error())
	}

	// Read in our policies
	rawAssumeRolePolicy, err := ioutil.ReadFile("assumeRolePolicy.json")
	if err != nil {
		return fmt.Errorf("unable to read assumeRolePolicy.json: %s", err.Error())
	}
	rawSyncPolicy, err := ioutil.ReadFile("syncPolicy.json")
	if err != nil {
		return fmt.Errorf("unable to read syncPolicy.json: %s", err.Error())
	}

	// Attempt to sts:AssumeRole into the target account.
	sess := session.Must(session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
	}))
	targetRoleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", config.TargetAccountID, config.TargetAccountExecutionRole)
	targetCreds := stscreds.NewCredentials(sess, targetRoleARN)

	// Create the IAM role
	assumeRolePolicy := fmt.Sprintf(string(rawAssumeRolePolicy), externalID)
	targetRole := iam.CreateRoleInput{
		RoleName:                 aws.String(config.TargetRoleName),
		AssumeRolePolicyDocument: aws.String(assumeRolePolicy),
		Description:              aws.String("F5 AIP EC2 Integration"),
	}

	svc := iam.New(sess, &aws.Config{Credentials: targetCreds})
	_, err = svc.CreateRole(&targetRole)
	if err != nil {
		return err
	}

	// Create the Sync Policy
	targetRolePolicy := iam.CreatePolicyInput{
		PolicyName:     aws.String("f5-aip-ec2-sync"),
		PolicyDocument: aws.String(string(rawSyncPolicy)),
	}
	_, err = svc.CreatePolicy(&targetRolePolicy)
	if err != nil {
		return err
	}

	// Attach the Sync Policy to the IAM role
	attachSyncPolicy := iam.AttachRolePolicyInput{
		PolicyArn: aws.String(fmt.Sprintf("arn:aws:iam::%s:policy/f5-aip-ec2-sync", config.TargetAccountID)),
		RoleName:  aws.String(config.TargetRoleName),
	}
	_, err = svc.AttachRolePolicy(&attachSyncPolicy)
	if err != nil {
		return err
	}
	fmt.Printf("Successfully created TS Integration with EC2 Sync in TS org %s using ARN arn:aws:iam::%s:role/%s", config.F5OrgID, config.TargetAccountID, config.TargetRoleName)

	return nil
}

// determineAccountID - use the information in the event that AWS EventBridge sends us to determine the account
// we're going to sts:AssumeRole into to set up the IAM role.
func determineAccountID(e eventBridgeEvent) (target string, err error) {
	if e.Detail.EventSource == "controltower.amazonaws.com" && e.Detail.EventName == "CreateManagedAccount" {
		target = e.Detail.ServiceEventDetails.CreateManagedAccountStatus.Account.AccountID
	} else if e.Detail.EventSource == "organizations.amazonaws.com" && e.Detail.EventName == "CreateAccountResult" {
		target = e.Detail.ServiceEventDetails.CreateAccountStatus.AccountID
	} else {
		return "", fmt.Errorf("unable to determine target account: unknown EventSource/EventName")
	}
	return target, nil
}

// setupF5Integration - Send a request to the F5 AIP API to set up a new integration, and return the
// external ID for that integration so we can use it to set up an IAM role.
func setupF5Integration(config config) (string, error) {
	f5APICreds := f5aip.Config{
		User: config.F5UserID,
		Key:  config.F5APIKey,
		Org:  config.F5OrgID,
	}

	externalID, integrationID, err := f5AWSSetup(config.F5APIPath, f5APICreds, config.TargetAccountID, config.TargetRoleName)
	if err != nil {
		return "", fmt.Errorf("unable to set up AWS integration: %s", err.Error())
	}
	err = f5EC2SyncSetup(config.F5APIPath, f5APICreds, integrationID, config.EC2SyncRegions)
	if err != nil {
		return "", fmt.Errorf("unable to set up EC2 Sync: %s", err.Error())
	}

	return externalID, nil
}

func f5AWSSetup(baseEndpoint string, creds f5aip.Config, targetAccountID string, targetRole string) (string, string, error) {
	var intResp newIntegrationResp
	description := fmt.Sprintf("AWS %s", targetAccountID)
	client := &http.Client{}
	endpoint := baseEndpoint + "/v2/integrations/aws"
	payload := newIntegrationPayload{
		ARN:         fmt.Sprintf("arn:aws:iam::%s:role/%s", targetAccountID, targetRole),
		Description: description,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", "", fmt.Errorf("unable to serialize new integraton payload: %s", err.Error())
	}
	req, _ := f5aip.Request(creds, "POST", endpoint, payloadBytes)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		rawJSON, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", "", err
		}
		err = json.Unmarshal([]byte(rawJSON), &intResp)
		if err != nil {
			return "", "", err
		}
	} else {
		raw, _ := ioutil.ReadAll(resp.Body)
		return "", "", fmt.Errorf("got %d from api, body: %s", resp.StatusCode, string(raw))
	}
	return intResp.ExternalID, intResp.IntegrationID, nil
}

func f5EC2SyncSetup(baseEndpoint string, creds f5aip.Config, integrationID string, enabledRegions []string) error {
	client := &http.Client{}
	endpoint := baseEndpoint + "/v2/integrations/aws/" + integrationID + "/ec2"
	payload := newEC2SyncPayload{
		Enabled: true,
		Regions: enabledRegions,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("unable to serialize new integraton payload: %s", err.Error())
	}
	req, _ := f5aip.Request(creds, "PUT", endpoint, payloadBytes)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 204 {
		return nil
	} else {
		raw, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("got %d from api, body: %s", resp.StatusCode, string(raw))
	}
}
