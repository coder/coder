// Live-page probe: count and time the geometry APIs that force style and
// layout, per benchmark phase. Installs window.__bench.probe/probeReset,
// which bench.mjs calls at phase boundaries and stores as raw.probe.
//
//   node scripts/chatbench/bench.mjs ... --prep=scripts/chatbench/prep/geometry-probe.js
(() => {
	const stats = {};
	const bump = (name, ms) => {
		const s = (stats[name] ??= { calls: 0, ms: 0 });
		s.calls += 1;
		s.ms += ms;
	};
	const wrapGetter = (proto, prop) => {
		const desc = Object.getOwnPropertyDescriptor(proto, prop);
		if (!desc?.get) return `no getter ${prop}`;
		Object.defineProperty(proto, prop, {
			...desc,
			get() {
				const t = performance.now();
				const v = desc.get.call(this);
				bump(prop, performance.now() - t);
				return v;
			},
		});
		return prop;
	};
	const wrapMethod = (obj, prop, label = prop) => {
		const orig = obj[prop];
		if (typeof orig !== "function") return `no method ${label}`;
		obj[prop] = function (...args) {
			const t = performance.now();
			const v = orig.apply(this, args);
			bump(label, performance.now() - t);
			return v;
		};
		return label;
	};
	const installed = [
		wrapGetter(Element.prototype, "scrollHeight"),
		wrapGetter(Element.prototype, "clientHeight"),
		wrapGetter(Element.prototype, "scrollTop"),
		wrapGetter(HTMLElement.prototype, "offsetHeight"),
		wrapGetter(HTMLElement.prototype, "offsetTop"),
		wrapMethod(Element.prototype, "getBoundingClientRect"),
		wrapMethod(Element.prototype, "scrollTo", "Element.scrollTo"),
		wrapMethod(window, "getComputedStyle"),
	];
	// Count observer deliveries and animation frames to express the rest
	// per frame.
	const RO = window.ResizeObserver;
	window.ResizeObserver = class extends RO {
		constructor(cb) {
			super((entries, obs) => {
				bump("ResizeObserver.callback", 0);
				return cb(entries, obs);
			});
		}
	};
	const raf = window.requestAnimationFrame.bind(window);
	window.requestAnimationFrame = (cb) =>
		raf((ts) => {
			bump("rAF.callback", 0);
			return cb(ts);
		});
	window.addEventListener("scroll", () => bump("scroll.event", 0), {
		capture: true,
		passive: true,
	});

	window.__bench.probeReset = () => {
		for (const k of Object.keys(stats)) delete stats[k];
	};
	window.__bench.probe = () => {
		const out = {};
		for (const [k, s] of Object.entries(stats)) {
			out[k] = { calls: s.calls, ms: Math.round(s.ms) };
		}
		return out;
	};
	window.__prepResult = { installed };
})();
