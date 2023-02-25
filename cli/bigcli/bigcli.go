// Package bigcli offers an all-in-one solution for a highly configurable CLI
// application. Within Coder, we use it for our `server` subcommand, which
// demands more than cobra/viper can offer.
//
// We may extend its usage to the rest of our application, completely replacing
// cobra/viper, in the future. It's also a candidate to be broken out into its
// own open-source library.
package bigcli
