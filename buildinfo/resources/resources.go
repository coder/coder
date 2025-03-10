// This package is used for embedding .syso resource files into the binary
// during build and does not contain any code. During build, .syso files will be
// dropped in this directory and then removed after the build completes.
//
// This package must be imported by all binaries for this to work.
//
// See build_go.sh for more details.
package resources
