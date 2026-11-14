/**
 * `ended` means the server closed a command session, normally because its
 * command exited. Unlike `disconnected`, the terminal does not reconnect on
 * its own.
 */
export type ConnectionStatus =
	| "connected"
	| "disconnected"
	| "ended"
	| "initializing";
