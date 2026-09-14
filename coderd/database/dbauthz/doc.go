// Package dbauthz provides an authorization layer on top of the database. This
// package exposes an interface that is currently a 1:1 mapping with
// database.Store.
//
// The same cultural rules apply to this package as they do to database.Store.
// Meaning that each method implemented should keep the number of database
// queries as close to 1 as possible. Each method should do 1 thing, with no
// unexpected side effects (eg: updating multiple tables in a single method).
//
// Do not implement business logic in this package. Only authorization related
// logic should be implemented here. In most cases, this should only be a call to
// the rbac authorizer.
//
// When a new database method is added to database.Store, it should be added to
// this package as well. The unit test "Accounting" will ensure all methods are
// tested. See other unit tests for examples on how to write these.
package dbauthz
