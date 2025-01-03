// Package apiversion provides an API version type that can be used to validate
// compatibility between two API versions.
//
// NOTE: API VERSIONS ARE NOT SEMANTIC VERSIONS.
//
// API versions are represented as major.minor where major and minor are both
// positive integers.
//
// API versions are not directly tied to a specific release of the software.
// Instead, they are used to represent the capabilities of the server. For
// example, a server that supports API version 1.2 should be able to handle
// requests from clients that support API version 1.0, 1.1, or 1.2.
// However, a server that supports API version 2.0 is not required to handle
// requests from clients that support API version 1.x.
// Clients may need to negotiate with the server to determine the highest
// supported API version.
//
// When making a change to the API, use the following rules to determine the
// next API version:
//  1. If the change is backward-compatible, increment the minor version.
//     Examples of backward-compatible changes include adding new fields to
//     a response or adding new endpoints.
//  2. If the change is not backward-compatible, increment the major version.
//     Examples of non-backward-compatible changes include removing or renaming
//     fields.
package apiversion
