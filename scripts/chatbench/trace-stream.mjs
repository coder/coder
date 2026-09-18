// Stream a Chrome trace JSON file and yield each event object in the
// traceEvents array without materializing the whole file. Traces with
// invalidation tracking can exceed V8's string limit.
import fs from "node:fs";

export async function* traceEventsOf(file, keep = () => true) {
	const stream = fs.createReadStream(file, { highWaterMark: 1 << 22 });
	let buf = "";
	let pos = 0; // next index in buf to scan
	let inArray = false;
	let depth = 0;
	let start = -1; // index in buf of the current top-level object
	let inString = false;
	let escaped = false;
	for await (const chunk of stream) {
		buf += chunk.toString("utf8");
		if (!inArray) {
			const k = buf.indexOf('"traceEvents"');
			if (k < 0) {
				buf = buf.slice(-20);
				pos = 0;
				continue;
			}
			pos = buf.indexOf("[", k) + 1;
			inArray = true;
		}
		for (; pos < buf.length; pos++) {
			const c = buf[pos];
			if (inString) {
				if (escaped) escaped = false;
				else if (c === "\\") escaped = true;
				else if (c === '"') inString = false;
				continue;
			}
			if (c === '"') inString = true;
			else if (c === "{") {
				if (depth === 0) start = pos;
				depth++;
			} else if (c === "}") {
				depth--;
				if (depth === 0 && start >= 0) {
					const text = buf.slice(start, pos + 1);
					start = -1;
					let ev;
					try {
						ev = JSON.parse(text);
					} catch {
						continue;
					}
					if (keep(ev)) yield ev;
				}
			} else if (c === "]" && depth === 0) {
				return;
			}
		}
		// Drop consumed text, keeping any unfinished object.
		if (depth === 0) {
			buf = "";
			pos = 0;
		} else {
			buf = buf.slice(start);
			pos -= start;
			start = 0;
		}
	}
}
