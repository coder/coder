// Live-page probe: per benchmark phase, counts markdown code blocks that
// mounted and <style> elements that were inserted. Installs
// window.__bench.probe/probeReset, which bench.mjs stores as raw.probe.
//
//   node scripts/chatbench/bench.mjs ... --prep=scripts/chatbench/prep/code-block-probe.js
(() => {
	let counts = { codeBlocks: 0, styles: 0 };
	const visit = (node) => {
		if (node.nodeType !== Node.ELEMENT_NODE) return;
		if (node.tagName === "STYLE") counts.styles++;
		counts.codeBlocks += node.matches('[class~="group/code-block"]') ? 1 : 0;
		counts.codeBlocks += node.querySelectorAll(
			'[class~="group/code-block"]',
		).length;
		counts.styles += node.querySelectorAll("style").length;
	};
	new MutationObserver((records) => {
		for (const r of records) for (const n of r.addedNodes) visit(n);
	}).observe(document, { childList: true, subtree: true });
	window.__bench.probe = () => ({ ...counts });
	window.__bench.probeReset = () => {
		counts = { codeBlocks: 0, styles: 0 };
	};
})();
