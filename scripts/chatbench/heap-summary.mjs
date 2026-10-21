// Summarize a V8 heap snapshot (.heapsnapshot, as written by bench.mjs
// --heap=1) without loading it as one JSON value; snapshots of this app
// are several hundred MB. Prints shallow size and count per node class,
// the largest strings, and closures by function name.
//
//   node scripts/chatbench/heap-summary.mjs <file.heapsnapshot> [topN] [filter] [--shapes]
//
// filter is a case-insensitive substring; when given, also prints the
// largest individual nodes whose class or string content matches it.
// --shapes groups plain Object nodes by their property names (first 8,
// sorted) using the edge table, which tells React props, markdown AST
// nodes and other anonymous objects apart.
import fs from "node:fs";

const argv = process.argv.slice(2);
const wantShapes = argv.includes("--shapes");
const [file, topArg = "40", filter] = argv.filter((a) => a !== "--shapes");
const topN = Number(topArg);

// Sequential scanner over the file. Every array we need is a flat JSON
// array of integers or of strings, so a small hand-written tokenizer is
// enough and avoids materializing the document.
class Scanner {
	constructor(path) {
		this.fd = fs.openSync(path, "r");
		this.size = fs.fstatSync(this.fd).size;
		this.buf = Buffer.alloc(16 * 2 ** 20);
		this.pos = 0; // absolute offset of buf[0]
		this.len = 0;
		this.i = 0; // index into buf
		this.fill();
	}
	fill() {
		const keep = this.len - this.i;
		if (keep > 0) this.buf.copy(this.buf, 0, this.i, this.len);
		this.pos += this.i;
		this.len = keep;
		this.i = 0;
		const n = fs.readSync(
			this.fd,
			this.buf,
			this.len,
			this.buf.length - this.len,
			this.pos + this.len,
		);
		this.len += n;
		return n > 0;
	}
	peek() {
		if (this.i >= this.len && !this.fill()) return -1;
		return this.buf[this.i];
	}
	next() {
		const c = this.peek();
		if (c >= 0) this.i++;
		return c;
	}
	// Advances past the next occurrence of the ASCII needle.
	seek(needle) {
		const nb = Buffer.from(needle);
		for (;;) {
			const idx = this.buf.indexOf(nb, this.i);
			if (idx >= 0 && idx + nb.length <= this.len) {
				this.i = idx + nb.length;
				return true;
			}
			// Keep a tail so a needle split across chunks is still found.
			this.i = Math.max(this.i, this.len - nb.length);
			if (!this.fill()) return false;
		}
	}
	skipWs() {
		for (;;) {
			const c = this.peek();
			if (c === 0x20 || c === 0x0a || c === 0x0d || c === 0x09) this.i++;
			else return;
		}
	}
	// Reads a JSON array of integers into a typed array.
	readIntArray(count, width) {
		const out = new (width === 64 ? Float64Array : Uint32Array)(count);
		this.skipWs();
		if (this.next() !== 0x5b) throw new Error("expected [");
		let n = 0;
		let cur = 0;
		let inNum = false;
		for (;;) {
			const c = this.next();
			if (c >= 0x30 && c <= 0x39) {
				cur = cur * 10 + (c - 0x30);
				inNum = true;
			} else {
				if (inNum) {
					out[n++] = cur;
					cur = 0;
					inNum = false;
				}
				if (c === 0x5d || c < 0) break;
			}
		}
		if (n !== count) throw new Error(`expected ${count} ints, read ${n}`);
		return out;
	}
	// Reads a JSON array of strings.
	readStringArray() {
		const out = [];
		this.skipWs();
		if (this.next() !== 0x5b) throw new Error("expected [");
		for (;;) {
			this.skipWs();
			const c = this.next();
			if (c === 0x5d || c < 0) break;
			if (c === 0x2c) continue;
			if (c !== 0x22)
				throw new Error(`expected string at ${this.pos + this.i}`);
			const bytes = [];
			for (;;) {
				const d = this.next();
				if (d === 0x22) break;
				if (d === 0x5c) {
					const e = this.next();
					if (e === 0x75) {
						let code = 0;
						for (let k = 0; k < 4; k++)
							code = code * 16 + parseInt(String.fromCharCode(this.next()), 16);
						bytes.push(...Buffer.from(String.fromCharCode(code), "utf8"));
					} else {
						const map = {
							0x6e: 0x0a,
							0x74: 0x09,
							0x72: 0x0d,
							0x62: 0x08,
							0x66: 0x0c,
						};
						bytes.push(map[e] ?? e);
					}
				} else bytes.push(d);
			}
			out.push(Buffer.from(bytes).toString("utf8"));
		}
		return out;
	}
	// Reads a JSON value that fits comfortably in memory (the header).
	// The value ends right before endNeedle, minus the separator.
	readSmallJson(endNeedle) {
		const start = this.pos + this.i;
		if (!this.seek(endNeedle)) throw new Error(`missing ${endNeedle}`);
		const end = this.pos + this.i - Buffer.byteLength(endNeedle);
		const b = Buffer.alloc(end - start);
		fs.readSync(this.fd, b, 0, b.length, start);
		return JSON.parse(b.toString("utf8").replace(/,\s*$/, ""));
	}
}

const s = new Scanner(file);
if (!s.seek('"snapshot":')) throw new Error("no snapshot header");
const snapshot = s.readSmallJson('"nodes":');
const { node_fields: nodeFields, node_types: nodeTypes } = snapshot.meta;
const nodeCount = snapshot.node_count;
const edgeCount = snapshot.edge_count;
const NF = nodeFields.length;
const fType = nodeFields.indexOf("type");
const fName = nodeFields.indexOf("name");
const fSize = nodeFields.indexOf("self_size");
const typeNames = nodeTypes[fType];
console.log(
	`${file}: ${nodeCount} nodes, ${edgeCount} edges, ${Math.round(s.size / 2 ** 20)} MB`,
);
const nodes = s.readIntArray(nodeCount * NF, 32);
if (!s.seek('"edges":')) throw new Error("no edges");
const EF = snapshot.meta.edge_fields.length;
const edgeTypeNames = snapshot.meta.edge_types[0];
const propertyEdge = edgeTypeNames.indexOf("property");
const fEdgeCount = nodeFields.indexOf("edge_count");
const edges = s.readIntArray(edgeCount * EF, 32);
if (!s.seek('"strings":')) throw new Error("no strings");
const strings = s.readStringArray();
console.log(`${strings.length} strings in table`);

const groups = new Map();
const add = (key, size) => {
	const g = groups.get(key) ?? { count: 0, size: 0 };
	g.count++;
	g.size += size;
	groups.set(key, g);
};
const bigStrings = [];
const matches = [];
const shapes = new Map();
let total = 0;
let edgeCursor = 0;
for (let n = 0; n < nodeCount; n++) {
	const base = n * NF;
	const type = typeNames[nodes[base + fType]];
	const name = strings[nodes[base + fName]] ?? "";
	const size = nodes[base + fSize];
	const nEdges = nodes[base + fEdgeCount];
	total += size;
	if (wantShapes && type === "object" && name === "Object") {
		const props = [];
		for (let e = 0; e < nEdges; e++) {
			const eb = (edgeCursor + e) * EF;
			if (edges[eb] === propertyEdge) props.push(strings[edges[eb + 1]] ?? "?");
		}
		props.sort();
		const sig = `{${props.slice(0, 8).join(",")}${props.length > 8 ? ",..." : ""}}`;
		const g = shapes.get(sig) ?? { count: 0, size: 0 };
		g.count++;
		g.size += size;
		shapes.set(sig, g);
	}
	edgeCursor += nEdges;
	let key;
	switch (type) {
		case "string":
		case "concatenated string":
		case "sliced string":
			key = `(${type})`;
			if (size >= 16 * 1024)
				bigStrings.push({ size, preview: name.slice(0, 100) });
			if (filter && name.toLowerCase().includes(filter.toLowerCase()))
				matches.push({ size, type, name: name.slice(0, 160) });
			break;
		case "closure":
			key = `closure ${name}`;
			break;
		case "object":
			key = name;
			break;
		default:
			key = `(${type}) ${name}`;
	}
	add(key, size);
	if (
		filter &&
		type !== "string" &&
		key.toLowerCase().includes(filter.toLowerCase())
	)
		matches.push({ size, type, name: key });
}
const mb = (b) => (b / 2 ** 20).toFixed(1);
console.log(`total shallow size ${mb(total)} MB\n`);
console.log("top classes by shallow size:");
for (const [key, g] of [...groups]
	.sort((a, b) => b[1].size - a[1].size)
	.slice(0, topN))
	console.log(
		`  ${mb(g.size).padStart(8)} MB ${String(g.count).padStart(9)} x ${key.slice(0, 90)}`,
	);
console.log("\nlargest strings:");
for (const b of bigStrings.sort((a, b) => b.size - a.size).slice(0, 15))
	console.log(`  ${mb(b.size).padStart(8)} MB ${JSON.stringify(b.preview)}`);
if (filter) {
	console.log(`\nlargest nodes matching ${JSON.stringify(filter)}:`);
	for (const m of matches.sort((a, b) => b.size - a.size).slice(0, topN))
		console.log(
			`  ${mb(m.size).padStart(8)} MB ${m.type} ${JSON.stringify(m.name)}`,
		);
}
if (wantShapes) {
	console.log("\nplain Object shapes by shallow size:");
	for (const [sig, g] of [...shapes]
		.sort((a, b) => b[1].size - a[1].size)
		.slice(0, topN))
		console.log(
			`  ${mb(g.size).padStart(8)} MB ${String(g.count).padStart(9)} x ${sig.slice(0, 120)}`,
		);
}
