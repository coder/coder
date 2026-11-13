/**
 * `ended` means a command session ended, normally because its command exited.
 * Unlike `disconnected`, the terminal does not reconnect on its own.
 */
export type ConnectionStatus =
	| "connected"
	| "disconnected"
	| "ended"
	| "initializing";
