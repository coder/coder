package derphealth

// DERP healthcheck is kept in a separate package as it is used by `cli/netcheck.go`,
// which is part of the slim binary. Slim binary can't have dependency on `database`,
// which is used by the database healthcheck.
