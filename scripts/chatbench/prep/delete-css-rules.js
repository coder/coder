// --prep helper: installs window.__deleteRules(pattern) for later prep steps.
// Deletes stylesheet rules whose selector matches `pattern`; returns counts.
// Walks nested group rules (@media, @supports, @layer) too.
window.__deleteRules = (pattern) => {
	let deleted = 0;
	let seen = 0;
	const walk = (group) => {
		let rules;
		try {
			rules = group.cssRules;
		} catch {
			return;
		}
		for (let i = rules.length - 1; i >= 0; i--) {
			const r = rules[i];
			if (r.cssRules?.length) walk(r);
			if (r.selectorText !== undefined) {
				seen++;
				if (pattern.test(r.selectorText)) {
					group.deleteRule(i);
					deleted++;
				}
			}
		}
	};
	for (const s of document.styleSheets) walk(s);
	return { seen, deleted };
};
