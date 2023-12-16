// Package gentest contains tests that are run at db generate time. These tests
// need to exist in their own package to avoid importing stuff that gets
// generated after the DB.
//
// E.g. if we put these tests in coderd/database, then we'd be importing dbmock
// which is generated after the DB and can cause type problems when building
// the tests.
package gentest
