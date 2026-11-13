// Live-page probe: per benchmark phase, records each height change of the
// chat composer's textbox and each style attribute write on <html>, with
// page times. Installs window.__bench.probe/probeReset, which bench.mjs
// stores as raw.probe. With --type-chars, a growth marks the keystroke that
// wrapped the composer onto a new line.
//
//   node scripts/chatbench/bench.mjs --scenario=single --stream=0 \
//     --only=idle-type --type-prefill=340 --type-chars=560 \
//     --prep=scripts/chatbench/prep/wrap-probe.js
(() => {
	const box = document.querySelector(
		'[role="textbox"][aria-label="Chat message"]',
	);
	let growths = [];
	let rootWrites = [];
	let lastH = box ? box.getBoundingClientRect().height : -1;
	if (box)
		new ResizeObserver(() => {
			const h = Math.round(box.getBoundingClientRect().height);
			if (h !== lastH) {
				growths.push({ t: Math.round(performance.now()), h });
				lastH = h;
			}
		}).observe(box);
	new MutationObserver((records) => {
		for (const _ of records) rootWrites.push(Math.round(performance.now()));
	}).observe(document.documentElement, {
		attributes: true,
		attributeFilter: ["style"],
	});
	window.__bench.probe = () => ({
		found: !!box,
		growths,
		rootWrites,
		h0: Math.round(lastH),
	});
	window.__bench.probeReset = () => {
		growths = [];
		rootWrites = [];
	};
	window.__prepResult = {
		found: !!box,
		h: Math.round(lastH),
		w: box ? Math.round(box.getBoundingClientRect().width) : -1,
	};
})();
