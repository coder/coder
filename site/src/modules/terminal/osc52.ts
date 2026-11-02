const ESC = 0x1b;
const BEL = 0x07;
const BACKSLASH = 0x5c;
const CAN = 0x18;
const SUB = 0x1a;
// "]52;" follows ESC to start an OSC 52 sequence.
const OSC52_PREFIX = [0x5d, 0x35, 0x32, 0x3b];
// Larger payloads are discarded so a runaway sequence cannot grow memory
// without bound.
const MAX_OSC52_PAYLOAD_LENGTH = 1024 * 1024;

type ParserState = "ground" | "prefix" | "payload" | "payloadEscape";

/**
 * Extracts OSC 52 clipboard writes, which programs like tmux use to copy to
 * the host clipboard, from terminal output. ghostty-web has no OSC handler
 * API, so output is scanned before it reaches the terminal. Sequences may be
 * split across chunks.
 */
export class Osc52Parser {
	#state: ParserState = "ground";
	#prefixIndex = 0;
	#payload = "";

	/**
	 * Feeds a chunk of terminal output and returns the decoded text of every
	 * OSC 52 clipboard write completed in this chunk.
	 */
	feed(chunk: string | Uint8Array): string[] {
		const results: string[] = [];
		for (let i = 0; i < chunk.length; i++) {
			const code = typeof chunk === "string" ? chunk.charCodeAt(i) : chunk[i];
			const decoded = this.#consume(code);
			if (decoded !== undefined) {
				results.push(decoded);
			}
		}
		return results;
	}

	#consume(code: number): string | undefined {
		switch (this.#state) {
			case "ground":
				if (code === ESC) {
					this.#reset("prefix");
				}
				return undefined;
			case "prefix":
				if (code === OSC52_PREFIX[this.#prefixIndex]) {
					this.#prefixIndex++;
					if (this.#prefixIndex === OSC52_PREFIX.length) {
						this.#state = "payload";
					}
				} else if (code === ESC) {
					this.#reset("prefix");
				} else {
					this.#reset();
				}
				return undefined;
			case "payload":
				if (code === BEL) {
					return this.#finish();
				}
				if (code === ESC) {
					this.#state = "payloadEscape";
					return undefined;
				}
				if (code === CAN || code === SUB) {
					this.#reset();
					return undefined;
				}
				if (this.#payload.length >= MAX_OSC52_PAYLOAD_LENGTH) {
					this.#reset();
					return undefined;
				}
				this.#payload += String.fromCharCode(code);
				return undefined;
			case "payloadEscape":
				if (code === BACKSLASH) {
					return this.#finish();
				}
				// Any other byte aborts the sequence. The ESC may begin a new one.
				this.#reset("prefix");
				return this.#consume(code);
		}
	}

	#reset(state: ParserState = "ground") {
		this.#state = state;
		this.#prefixIndex = 0;
		this.#payload = "";
	}

	#finish(): string | undefined {
		const payload = this.#payload;
		this.#reset();
		return decodeOsc52Payload(payload);
	}
}

/**
 * Decodes the `<selection>;<base64>` portion of an OSC 52 sequence. Returns
 * undefined for clipboard read queries and malformed payloads.
 */
const decodeOsc52Payload = (payload: string): string | undefined => {
	const separatorIndex = payload.indexOf(";");
	if (separatorIndex === -1) {
		return undefined;
	}
	const base64 = payload.slice(separatorIndex + 1);
	// A "?" payload is a clipboard read query. Responding would require
	// writing back to the PTY, which is not supported.
	if (!base64 || base64 === "?") {
		return undefined;
	}
	try {
		const bytes = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0));
		return new TextDecoder().decode(bytes);
	} catch {
		// Invalid base64.
		return undefined;
	}
};
