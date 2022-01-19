package srverr

import (
	"net/http"
)

// SettableError describes a structured error that can accept an error. This is
// useful to prevent handlers from needing to insert the error into Upgrade
// twice. xjson.HandleError uses this interface set the final error string
// before marshaling.
type VerboseError interface {
	SetVerbose(err error)
}

// Verbose is for reusing the `verbose` field between error types. It
// implements VerboseError so it's not necessary to prefill the struct with the
// verbose error.
type Verbose struct {
	Verbose string `json:"verbose"`
}

func (e *Verbose) SetVerbose(err error) { e.Verbose = err.Error() }

// Code is a string enum indicating the structure of the details field in an
// error response. Each error type should correspond to a unique Code.
type Code string

const (
	CodeServerError      Code = "server_error"
	CodeDatabaseError    Code = "database_error"
	CodeResourceNotFound Code = "resource_not_found"
)

var _ VerboseError = &ServerError{}

// ServerError describes an error of unknown origins.
type ServerError struct {
	Verbose
}

func (*ServerError) Status() int           { return http.StatusInternalServerError }
func (*ServerError) PublicMessage() string { return "An internal server error occurred." }
func (*ServerError) Code() Code            { return CodeServerError }
func (*ServerError) Error() string         { return "internal server error" }

// DatabaseError describes an unknown error from the database.
type DatabaseError struct {
	Verbose
}

func (*DatabaseError) Status() int           { return http.StatusInternalServerError }
func (*DatabaseError) PublicMessage() string { return "A database error occurred." }
func (*DatabaseError) Code() Code            { return CodeDatabaseError }
func (*DatabaseError) Error() string         { return "database error" }

// ResourceNotFoundError describes an error when a provided resource ID was not
// found within the database or the user does not have the proper permission to
// view it.
type ResourceNotFoundError struct {
}

func (ResourceNotFoundError) Status() int             { return http.StatusNotFound }
func (e ResourceNotFoundError) PublicMessage() string { return "Resource not found." }
func (ResourceNotFoundError) Code() Code              { return CodeResourceNotFound }
func (ResourceNotFoundError) Error() string           { return "resource not found" }
