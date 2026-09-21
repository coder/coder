// Summarize a React DevTools profile saved by bench.mjs --react=1: every
// commit with its render duration, effect durations, priority, the
// components that scheduled it (updaters) and the components with the
// most self time, plus per-component totals for the whole phase. Commit
// times are page times, so they line up with the interaction log.
//
//   node scripts/chatbench/react-summary.mjs <phase.reactprofile.json> [minCommitMs] [topN]
import fs from "node:fs";

const [file, minMsArg = "0", topNArg = "6"] = process.argv.slice(2);
const minMs = Number(minMsArg);
const topN = Number(topNArg);
const d = JSON.parse(fs.readFileSync(file, "utf8"));

console.log(`${d.phase}: ${d.interactions.length} interactions`);
for (const i of d.interactions)
	console.log(
		`  @${i.t0}ms ${i.type}${i.key ? ` ${i.key}` : ""} latency ${i.latency}ms`,
	);

for (const renderer of d.renderers) {
	const name = (id) => renderer.names[id] ?? `#${id}`;
	for (const root of renderer.dataForRoots) {
		const commits = root.commitData;
		const totals = new Map();
		let renderTotal = 0;
		console.log(
			`\nrenderer ${renderer.rendererID} root ${root.displayName}: ${commits.length} commits`,
		);
		for (const c of commits) {
			renderTotal += c.duration;
			for (const [id, self] of c.fiberSelfDurations) {
				const k = name(id);
				const t = totals.get(k) ?? { n: 0, ms: 0 };
				t.n++;
				t.ms += self;
				totals.set(k, t);
			}
			if (c.duration < minMs) continue;
			const at = Math.round(renderer.profilingStart + c.timestamp);
			const updaters =
				(c.updaters ?? []).map((u) => `${u.displayName}#${u.id}`).join(", ") ||
				"-";
			const bySelf = new Map();
			for (const [id, self] of c.fiberSelfDurations)
				bySelf.set(name(id), (bySelf.get(name(id)) ?? 0) + self);
			const top = [...bySelf.entries()]
				.sort((a, b) => b[1] - a[1])
				.slice(0, topN)
				.map(([k, ms]) => `${k} ${ms.toFixed(1)}ms`)
				.join(" | ");
			const changed = c.changeDescriptions
				? c.changeDescriptions
						.filter(
							([, cd]) =>
								cd &&
								(cd.didHooksChange ||
									cd.props?.length ||
									cd.state?.length ||
									cd.context),
						)
						.slice(0, topN)
						.map(
							([id, cd]) =>
								`${name(id)}(${[cd.didHooksChange && "hooks", cd.props?.length && `props:${cd.props.join(",")}`, cd.state?.length && "state", cd.context && "context"].filter(Boolean).join(" ")})`,
						)
						.join(", ")
				: null;
			console.log(
				`  @${at}ms render ${c.duration.toFixed(1)}ms effects ${c.effectDuration ?? "-"}/${c.passiveEffectDuration ?? "-"}ms ${c.priorityLevel ?? ""} fibers=${c.fiberSelfDurations.length} updaters=[${updaters}]`,
			);
			console.log(`      top self: ${top}`);
			if (changed) console.log(`      why: ${changed}`);
		}
		console.log(
			`  total render time ${renderTotal.toFixed(1)}ms; components by self time:`,
		);
		for (const [k, t] of [...totals.entries()]
			.sort((a, b) => b[1].ms - a[1].ms)
			.slice(0, 15)) {
			console.log(
				`    ${t.ms.toFixed(1).padStart(8)}ms  x${String(t.n).padStart(5)}  ${k}`,
			);
		}
	}
}
