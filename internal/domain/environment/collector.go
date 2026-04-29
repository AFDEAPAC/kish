package environment

import (
	"context"
	"time"
)

// Collector is the interface that all environment collectors must implement.
//
// A collector is responsible for gathering one category of environment data.
// It must never terminate the entire detection process on failure; instead it
// should return a CollectorResult with an appropriate Status.
type Collector interface {
	// Name returns the unique identifier for this collector (e.g. "linux_system").
	Name() string

	// Collect performs the data collection and returns the result.
	// The provided context may carry a deadline or cancellation signal.
	Collect(ctx context.Context) CollectorResult
}

// CollectorResult is the internal result returned by a collector to the
// detection service. It is not serialized directly into the output JSON;
// the service merges its fields into the snapshot.
type CollectorResult struct {
	// Name is the collector identifier, matching Collector.Name().
	Name string

	// Status is the outcome of this collector run.
	Status CollectorStatus

	// StartedAt is when the collector began.
	StartedAt time.Time

	// FinishedAt is when the collector finished.
	FinishedAt time.Time

	// Data holds normalized key-value facts collected by this collector.
	Data map[string]string

	// PackageSets holds any package lists collected by this collector.
	PackageSets []PackageSet

	// Devices holds structured hardware device data produced by this collector.
	// The detection service merges this into the snapshot's Devices field.
	Devices *Devices

	// Raw holds unstructured or command-output data keyed by source name.
	// Package-command outputs should go into PackageSet.RawText instead.
	Raw map[string]string

	// Warnings records non-fatal issues encountered during collection.
	Warnings []string

	// Error describes a fatal collector error (if Status is failed).
	Error string
}

// CollectorReport is the summary of a collector's execution that appears
// in the top-level collectors field of the output snapshot.
//
// Raw outputs and full data are not included; they are merged into the
// snapshot's top-level fields by the detection service.
type CollectorReport struct {
	// Name is the collector identifier.
	Name string `json:"name"`

	// Status is the outcome of this collector run.
	Status CollectorStatus `json:"status"`

	// Warnings contains non-fatal issues from this collector.
	Warnings []string `json:"warnings,omitempty"`

	// Error describes a fatal error if Status is failed.
	Error string `json:"error,omitempty"`
}
