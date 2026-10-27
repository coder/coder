// Summarize idle-type runs made with --type-chars and prep/wrap-probe.js:
// for each composer growth, the keystroke that wrapped the text onto a new
// line, and the other keystrokes' p50. Only the typing window (between the
// type-start and type-end marks) counts, so the prefill and the final clear
// are left out.
//
// The probe's ResizeObserver reports a growth after the frame that rendered
// the wrap, so the wrapping keystroke is the slowest one in the window
// before the growth: --window=1000 suits Chromium, and WebKit needs 4000
// when a root restyle holds that frame for several seconds.
//
//   node scripts/chatbench/wrap-summary.mjs [--window=<ms>] <results.json>...
import fs from "node:fs";

const args = process.argv.slice(2);
const windowArg = args.find((a) => a.startsWith("--window="));
const back = Number(windowArg?.split("=")[1] ?? 1000);
const files = args.filter((a) => !a.startsWith("--"));

const median = (xs) => {
	const s = [...xs].sort((a, b) => a - b);
	return s.length ? s[Math.floor(s.length / 2)] : undefined;
};

const pooled = [];
for (const file of files) {
	const results = JSON.parse(fs.readFileSync(file, "utf8"));
	const raw = results.phases["idle-type"].raw;
	const marks = Object.fromEntries(raw.marks.map((m) => [m.name, m.t]));
	const t0 = marks["type-start"];
	const t1 = marks["type-end"];
	const inTyping = (t) => t >= t0 && t <= t1;
	const keys = raw.interactions.filter(
		(i) => i.type === "key" && i.key.length === 1 && inTyping(i.t0),
	);
	const growths = raw.probe.growths.filter((g) => inTyping(g.t));
	const wrap = new Set();
	for (const g of growths) {
		let slowest = -1;
		keys.forEach((k, j) => {
			if (k.t0 < g.t - back || k.t0 > g.t) return;
			if (slowest < 0 || k.latency > keys[slowest].latency) slowest = j;
		});
		if (slowest >= 0) wrap.add(slowest);
	}
	const wrapLatencies = [...wrap].map((j) => keys[j].latency);
	pooled.push(...wrapLatencies);
	const other = keys.filter((_, j) => !wrap.has(j)).map((k) => k.latency);
	const records = raw.probe.rootWrites.filter(inTyping).length;
	console.log(
		`${file}: ${keys.length} keys, ${growths.length} growths, ${records} <html> style mutation records;` +
			` wrap keys ${wrapLatencies.sort((a, b) => a - b).join("/")} ms; other keys p50 ${median(other)} ms`,
	);
}
if (pooled.length)
	console.log(
		`all wrap keys pooled: n=${pooled.length} min ${Math.min(...pooled)} ms max ${Math.max(...pooled)} ms median ${median(pooled)} ms`,
	);
