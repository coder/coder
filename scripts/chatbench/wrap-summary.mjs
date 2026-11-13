// Summarize idle-type runs made with --type-chars and prep/wrap-probe.js:
// for each composer growth, the keystroke that wrapped the text onto a new
// line, and the other keystrokes' p50. Only the typing window (between the
// type-start and type-end marks) counts, so the prefill and the final clear
// are left out.
//
// The wrapping keystroke is the one whose input changed the composer's
// height. On main, the dropdown effect writes the changed variables to
// <html> inside that keystroke's input handler, so the last keystroke before
// a growth's first <html> style mutation record is the wrapping one. Without
// records, the probe's ResizeObserver reports the growth in the frame that
// rendered the keystroke, so it is the last keystroke before the growth.
// The filler text is deterministic, so the printed positions should match
// across runs of one browser; exits non-zero if a growth pairs with no
// keystroke.
//
//   node scripts/chatbench/wrap-summary.mjs <results.json>...
import fs from "node:fs";

const median = (xs) => {
	const s = [...xs].sort((a, b) => a - b);
	return s.length ? s[Math.floor(s.length / 2)] : undefined;
};

const pooled = [];
let unpaired = 0;
for (const file of process.argv.slice(2)) {
	const results = JSON.parse(fs.readFileSync(file, "utf8"));
	const raw = results.phases["idle-type"].raw;
	const marks = Object.fromEntries(raw.marks.map((m) => [m.name, m.t]));
	const t0 = marks["type-start"];
	const t1 = marks["type-end"];
	const inTyping = (t) => t >= t0 && t <= t1;
	const keys = raw.interactions.filter(
		(i) => i.type === "key" && i.key.length === 1 && inTyping(i.t0),
	);
	const growths = raw.probe.growths.map((g) => g.t).filter(inTyping);
	const records = raw.probe.rootWrites.filter(inTyping);
	const lastKeyAtOrBefore = (t) => keys.findLastIndex((k) => k.t0 <= t);
	const positions = [];
	let previous = t0;
	for (const g of growths) {
		const record = records.find((r) => r > previous && r <= g);
		const position = lastKeyAtOrBefore(record ?? g);
		previous = g;
		if (position < 0) {
			console.error(`${file}: growth at ${g} ms pairs with no keystroke`);
			unpaired++;
			continue;
		}
		positions.push(position);
	}
	const wrap = new Set(positions);
	const wrapLatencies = positions.map((j) => keys[j].latency);
	pooled.push(...wrapLatencies);
	const other = keys.filter((_, j) => !wrap.has(j)).map((k) => k.latency);
	console.log(
		`${file}: ${keys.length} keys, ${growths.length} growths, ${records.length} <html> style mutation records;` +
			` wrap positions ${positions.join("/")}; wrap keys ${wrapLatencies.join("/")} ms; other keys p50 ${median(other)} ms`,
	);
}
if (pooled.length)
	console.log(
		`all wrap keys pooled: n=${pooled.length} min ${Math.min(...pooled)} ms max ${Math.max(...pooled)} ms median ${median(pooled)} ms`,
	);
if (unpaired) process.exit(1);
