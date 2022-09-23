// stacker - automatically integrate new AWS accounts in your AWS organization
//           with F5 Application Infrastructure Protection (AIP)
//
// structs.go: data structures used in stacker
//
// Copyright 2022 F5 Inc.
// Licensed under the Apache License, Version 2.0 (the "License"); see
// the LICENSE file in this repository for more information.

package main

// eventBridgeEvent - the main nutshell all the eventbridge events get shipped in
type eventBridgeEvent struct {
	Version    string            `json:"version"`
	ID         string            `json:"id"`
	DetailType string            `json:"detail-type"`
	Source     string            `json:"source"`
	Time       string            `json:"time"`
	Region     string            `json:"region"`
	Resources  []string          `json:"resources"`
	Detail     eventBridgeDetail `json:"detail"`
}

// eventBridgeDetail - struct for the Detail section. ServiceEventDetails is an interface
// to allow for different structs underneath
type eventBridgeDetail struct {
	EventVersion        string                  `json:"eventVersion"`
	UserIdentity        eventBridgeUserIdentity `json:"userIdentity"`
	EventTime           string                  `json:"eventTime"`
	EventSource         string                  `json:"eventSource"`
	EventName           string                  `json:"eventName"`
	AWSRegion           string                  `json:"awsRegion"`
	SourceIPAddress     string                  `json:"sourceIPAddress"`
	UserAgent           string                  `json:"userAgent"`
	EventID             string                  `json:"eventID"`
	ReadOnly            bool                    `json:"readOnly"`
	EventType           string                  `json:"eventType"`
	ServiceEventDetails serviceEventDetails     `json:"serviceEventDetails"`
}

// eventBridgeUserIdentity - Eventbridge events come with an identity
type eventBridgeUserIdentity struct {
	AccountID string `json:"accountId"`
	InvokedBy string `json:"invokedBy"`
}

// controlTowerServiceEvent - wrapper for the Control Tower Service Event Details
type serviceEventDetails struct {
	CreateManagedAccountStatus controlTowerServiceEventStatus `json:"createManagedAccountStatus"`
	CreateAccountStatus        orgCreateAccountStatus         `json:"createAccountStatus"`
}

// controlTowerServiceEventStatus - wrapper for Control Tower Create Account
type controlTowerServiceEventStatus struct {
	OrganizationalUnit controlTowerServiceEventOU      `json:"organizationalUnit"`
	Account            controlTowerServiceEventAccount `json:"account"`
	State              string                          `json:"state"`
	Message            string                          `json:"message"`
	RequestedTimestamp string                          `json:"requestedTimestamp"`
	CompletedTimestamp string                          `json:"completedTimestamp"`
}

// controlTowerServiceEventOU - Control Tower account creation includes OU info
type controlTowerServiceEventOU struct {
	OrganizationalUnitName string `json:"organizationalUnitName"`
	OrganizationalUnitID   string `json:"organizationalUnitId"`
}

// controlTowerServiceEventAccount - Control Tower account creation - ID info
type controlTowerServiceEventAccount struct {
	AccountName string `json:"accountName"`
	AccountID   string `json:"accountId"`
}

// orgCreateAccountStatus - the struct for info about a created org
type orgCreateAccountStatus struct {
	ID                 string `json:"id"`
	State              string `json:"state"`
	AccountName        string `json:"accountName"`
	AccountID          string `json:"AccountId"`
	RequestedTimestamp string `json:"requestedTimestamp"`
	CompletedTimestamp string `json:"completedTimestamp"`
}

// newIntegrationPayload - request to the new integration endpoint
type newIntegrationPayload struct {
	ARN         string `json:"arn"`
	Description string `json:"description"`
}

// newIntegrationResp - response from a new integration request
type newIntegrationResp struct {
	IntegrationID string `json:"id"`
	ARN           string `json:"arn"`
	ExternalID    string `json:"externalId"`
	Description   string `json:"description"`
}

// newEC2SyncPayload - request to new ec2sync setup endpoint
type newEC2SyncPayload struct {
	Enabled bool     `json:"enabled"`
	Regions []string `json:"regions"`
}
