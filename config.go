// stacker - automatically integrate new AWS accounts in your AWS organization
//           with F5 Application Infrastructure Protection (AIP)
//
// config.go: configuration fields and initialization
//
// Copyright 2022 F5 Inc.
// Licensed under the Apache License, Version 2.0 (the "License"); see
// the LICENSE file in this repository for more information.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
)

type config struct {
	EC2SyncRegions             []string
	TargetAccountExecutionRole string
	TargetAccountID            string
	TargetRoleName             string
	F5APIPath                  string
	F5APIKey                   string
	F5APIKeyPath               string
	F5OrgID                    string
	F5UserID                   string
}

func buildConfig() (conf config, err error) {
	var errs []string

	regions := os.Getenv("F5_EC2_REGIONS")
	if regions == "" {
		errs = append(errs, "missing $F5_EC2_REGIONS")
	} else {
		conf.EC2SyncRegions = strings.Split(regions, ",")
	}
	conf.TargetAccountExecutionRole = os.Getenv("F5_TARGET_ACCOUNT_EXECUTION_ROLE")
	if conf.TargetAccountExecutionRole == "" {
		errs = append(errs, "missing $F5_TARGET_ACCOUNT_EXECUTION_ROLE")
	}
	conf.TargetRoleName = os.Getenv("F5_TARGET_ROLE_NAME")
	if conf.TargetRoleName == "" {
		errs = append(errs, "missing $F5_TARGET_ROLE_NAME")
	}
	conf.F5APIKeyPath = os.Getenv("F5_API_KEY_PATH")
	if conf.F5APIKeyPath == "" {
		errs = append(errs, "missing $F5_API_KEY_PATH")
	}
	conf.F5OrgID = os.Getenv("F5_ORG_ID")
	if conf.F5OrgID == "" {
		errs = append(errs, "missing $F5_ORG_ID")
	}
	conf.F5UserID = os.Getenv("F5_USER_ID")
	if conf.F5UserID == "" {
		errs = append(errs, "missing $F5_USER_ID")
	}
	conf.F5APIPath = os.Getenv("F5_API_PATH")
	if conf.F5APIPath == "" {
		conf.F5APIPath = "https://api.threatstack.com"
	}

	if len(errs) > 0 {
		return conf, fmt.Errorf("%s", strings.Join(errs, ", "))
	}

	conf.F5APIKey, err = getSecret(conf.F5APIKeyPath)
	if err != nil {
		return conf, err
	}

	return conf, nil
}

func getSecret(secretName string) (string, error) {
	sess := session.Must(session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
	}))
	svc := ssm.New(sess)
	paramQuery := &ssm.GetParameterInput{
		Name:           aws.String(secretName),
		WithDecryption: aws.Bool(true),
	}
	result, err := svc.GetParameter(paramQuery)
	if err != nil {
		return "", err
	}

	return aws.StringValue(result.Parameter.Value), nil
}
