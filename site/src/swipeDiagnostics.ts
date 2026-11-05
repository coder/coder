// TEMPORARY, do not merge. Records navigation state and touch gestures for
// the iOS swipe-back problems (CODAGT-1474) to localStorage, and shows a
// "Diag" tab that copies the record, so one device run captures everything.

const storageKey = "swipe-diag";
const maxEntries = 3000;
const diagVersion = "swipe-diag-4";
const overscrollXKey = "swipe-diag-overscroll-x";
const transcriptSelector = '[role="region"][aria-label="Messages"]';
const session = Math.random().toString(36).slice(2, 8);

type Entry = { t: number; s: string; e: string; d?: unknown };
type NavEntry = {
	url: string | null;
	index: number;
	key: string;
	sameDocument: boolean;
};
type NavLike = EventTarget & {
	entries(): NavEntry[];
	currentEntry: NavEntry | null;
};
type NavigateEventLike = Event & {
	navigationType?: string;
	destination?: { url?: string; index?: number };
	hashChange?: boolean;
	userInitiated?: boolean;
	canIntercept?: boolean;
};

const nav = (window as Window & { navigation?: NavLike }).navigation;

const safeJSON = (value: unknown): string => {
	try {
		return JSON.stringify(value) ?? "undefined";
	} catch {
		return "unserializable";
	}
};

const readEntries = (): Entry[] => {
	try {
		const raw = localStorage.getItem(storageKey);
		return raw ? (JSON.parse(raw) as Entry[]) : [];
	} catch {
		return [];
	}
};

const record = (e: string, d?: unknown) => {
	try {
		const entries = readEntries();
		entries.push({ t: Date.now(), s: session, e, d });
		localStorage.setItem(
			storageKey,
			JSON.stringify(entries.slice(-maxEntries)),
		);
	} catch {
		// Recording must never break the app.
	}
};

const snapshot = () => ({
	path: location.pathname + location.search + location.hash,
	len: history.length,
	state: safeJSON(history.state),
	vis: document.visibilityState,
	focus: document.hasFocus(),
	navIndex: nav?.currentEntry?.index ?? null,
	navEntries: nav
		? nav.entries().map((entry) => `${entry.index}:${entry.url}`)
		: null,
});

const activation = () =>
	navigator.userActivation
		? {
				active: navigator.userActivation.isActive,
				been: navigator.userActivation.hasBeenActive,
			}
		: null;

const launchParams = new URLSearchParams(location.search);
const launchPath = location.pathname + location.search;
const launchHistoryLength = history.length;
const navTiming = performance.getEntriesByType("navigation")[0] as
	| PerformanceNavigationTiming
	| undefined;
record("boot", {
	...snapshot(),
	version: diagVersion,
	navType: navTiming?.type ?? null,
	referrer: document.referrer,
	opener: window.opener !== null,
	name: window.name,
	standalone: matchMedia("(display-mode: standalone)").matches,
	iosStandalone: (navigator as Navigator & { standalone?: boolean }).standalone,
	ua: navigator.userAgent,
	activation: activation(),
	controller: navigator.serviceWorker?.controller?.scriptURL ?? null,
	swipeTest: launchParams.get("swipe_test"),
	via: launchParams.get("via"),
	run: launchParams.get("run"),
	timeOrigin: performance.timeOrigin,
});
navigator.serviceWorker?.getRegistration().then(
	(registration) =>
		record("registration", {
			scope: registration?.scope ?? null,
			active: registration?.active?.scriptURL ?? null,
			waiting: registration?.waiting?.scriptURL ?? null,
			installing: registration?.installing?.scriptURL ?? null,
		}),
	(error) => record("registrationError", String(error)),
);

// Runs after the module graph, so after router.tsx rewrote history.
queueMicrotask(() => record("afterRouter", snapshot()));

for (const method of ["pushState", "replaceState"] as const) {
	const original = history[method].bind(history);
	history[method] = (state, unused, url) => {
		const lenBefore = history.length;
		original(state, unused, url);
		record(method, {
			url: url == null ? null : String(url),
			state: safeJSON(state),
			lenBefore,
			len: history.length,
			activation: activation(),
		});
	};
}

const listen = (
	target: EventTarget,
	type: string,
	details: (event: Event) => Record<string, unknown> = () => ({}),
) => {
	target.addEventListener(
		type,
		(event) => record(type, { ...snapshot(), ...details(event) }),
		{ capture: true, passive: true },
	);
};

listen(window, "popstate", (event) => ({
	eventState: safeJSON((event as PopStateEvent).state),
}));
listen(window, "hashchange");
listen(window, "pageshow", (event) => ({
	persisted: (event as PageTransitionEvent).persisted,
}));
listen(window, "pagehide", (event) => ({
	persisted: (event as PageTransitionEvent).persisted,
}));
listen(window, "pagereveal");
listen(window, "pageswap");
listen(window, "beforeunload");
listen(window, "unload");
// Window focus only: element focus events would flood the record.
for (const type of ["focus", "blur"]) {
	window.addEventListener(type, (event) => {
		if (event.target === window) {
			record(`window${type}`, snapshot());
		}
	});
}
listen(document, "visibilitychange");
listen(document, "freeze");
listen(document, "resume");
listen(window, "error", (event) => ({
	message: (event as ErrorEvent).message,
}));
listen(window, "unhandledrejection", (event) => ({
	reason: String((event as PromiseRejectionEvent).reason),
}));
if (nav) {
	listen(nav, "navigate", (event) => {
		const navigate = event as NavigateEventLike;
		return {
			navigationType: navigate.navigationType,
			destination: navigate.destination?.url,
			destinationIndex: navigate.destination?.index,
			hashChange: navigate.hashChange,
			userInitiated: navigate.userInitiated,
			canIntercept: navigate.canIntercept,
		};
	});
	listen(nav, "currententrychange", (event) => {
		const change = event as Event & {
			navigationType?: string;
			from?: NavEntry;
		};
		return { navigationType: change.navigationType, from: change.from?.index };
	});
	listen(nav, "navigatesuccess");
	listen(nav, "navigateerror");
}

// A/B switch for the transcript scroller's overscroll-behavior-x. The app
// sets "contain" on both axes (overscroll-contain); "auto" lets the scroller
// hand horizontal pans to its parent.
const overscrollStyle = document.createElement("style");
const overscrollX = () =>
	localStorage.getItem(overscrollXKey) === "auto" ? "auto" : "app default";
const applyOverscrollX = () => {
	overscrollStyle.textContent =
		overscrollX() === "auto"
			? `${transcriptSelector} { overscroll-behavior-x: auto !important; }`
			: "";
	if (!overscrollStyle.isConnected) {
		document.head.appendChild(overscrollStyle);
	}
};
applyOverscrollX();

const describeElement = (element: Element | null) => {
	if (!element) {
		return null;
	}
	const classes = [...element.classList].filter((name) =>
		/overflow|overscroll|touch|scroll|fixed|absolute|sticky/.test(name),
	);
	return [
		element.tagName.toLowerCase(),
		element.getAttribute("role") && `role=${element.getAttribute("role")}`,
		element.getAttribute("aria-label") &&
			`label=${element.getAttribute("aria-label")?.slice(0, 30)}`,
		element.getAttribute("data-testid") &&
			`testid=${element.getAttribute("data-testid")}`,
		classes.length > 0 && `.${classes.join(".")}`,
	]
		.filter(Boolean)
		.join(" ");
};

// The touch-action values and scroll containers that WebKit consults before
// it lets a back swipe that starts inside the content begin.
const pointInfo = (x: number, y: number) => {
	const touchActions: string[] = [];
	const scrollers: Record<string, unknown>[] = [];
	const hit = document.elementFromPoint(x, y);
	for (
		let element: Element | null = hit;
		element && element !== document.documentElement;
		element = element.parentElement
	) {
		const style = getComputedStyle(element);
		if (style.touchAction !== "auto") {
			touchActions.push(`${style.touchAction} on ${describeElement(element)}`);
		}
		const scrollsX = /auto|scroll/.test(style.overflowX);
		const scrollsY = /auto|scroll/.test(style.overflowY);
		if ((scrollsX || scrollsY) && scrollers.length < 4) {
			scrollers.push({
				element: describeElement(element),
				overflow: `${style.overflowX} ${style.overflowY}`,
				overscroll: `${style.overscrollBehaviorX} ${style.overscrollBehaviorY}`,
				scrollLeft: Math.round(element.scrollLeft),
				scrollWidth: element.scrollWidth,
				clientWidth: element.clientWidth,
				scrollTop: Math.round(element.scrollTop),
				scrollHeight: element.scrollHeight,
				clientHeight: element.clientHeight,
			});
		}
	}
	return { hit: describeElement(hit), touchActions, scrollers };
};

// Script scrolls of the transcript: scroll events while no finger is down
// and no fling is settling.
let touchesDown = 0;
let lastTouchEndAt = 0;
let transcriptScrollsWithoutTouch = 0;
let lastTranscriptScrollWithoutTouchAt = 0;
let transcriptScrollsInGesture = 0;
document.addEventListener(
	"scroll",
	(event) => {
		if (!(event.target instanceof Element)) {
			return;
		}
		if (!event.target.matches(transcriptSelector)) {
			return;
		}
		if (touchesDown > 0) {
			transcriptScrollsInGesture++;
		} else if (Date.now() - lastTouchEndAt > 1500) {
			transcriptScrollsWithoutTouch++;
			lastTranscriptScrollWithoutTouchAt = Date.now();
		}
	},
	{ capture: true, passive: true },
);

const transcriptInfo = () => {
	const transcript = document.querySelector(transcriptSelector);
	if (!transcript) {
		return null;
	}
	const style = getComputedStyle(transcript);
	return {
		scrollTop: Math.round(transcript.scrollTop),
		scrollHeight: transcript.scrollHeight,
		clientHeight: transcript.clientHeight,
		overscroll: `${style.overscrollBehaviorX} ${style.overscrollBehaviorY}`,
		scriptScrolls: transcriptScrollsWithoutTouch,
		msSinceScriptScroll: lastTranscriptScrollWithoutTouchAt
			? Date.now() - lastTranscriptScrollWithoutTouchAt
			: null,
	};
};

// One record per single-finger gesture, written when it ends.
type Gesture = {
	startedAt: number;
	x: number;
	y: number;
	lastX: number;
	lastY: number;
	maxDX: number;
	maxDY: number;
	start: ReturnType<typeof pointInfo>;
	transcript: ReturnType<typeof transcriptInfo>;
	path: string;
};
let gesture: Gesture | null = null;
window.addEventListener(
	"touchstart",
	(event) => {
		touchesDown = event.touches.length;
		const touch = event.touches[0];
		if (event.touches.length !== 1 || !touch) {
			gesture = null;
			return;
		}
		transcriptScrollsInGesture = 0;
		gesture = {
			startedAt: Date.now(),
			x: Math.round(touch.clientX),
			y: Math.round(touch.clientY),
			lastX: touch.clientX,
			lastY: touch.clientY,
			maxDX: 0,
			maxDY: 0,
			start: pointInfo(touch.clientX, touch.clientY),
			transcript: transcriptInfo(),
			path: location.pathname,
		};
	},
	{ capture: true, passive: true },
);
window.addEventListener(
	"touchmove",
	(event) => {
		const touch = event.touches[0];
		if (!gesture || !touch) {
			return;
		}
		gesture.lastX = touch.clientX;
		gesture.lastY = touch.clientY;
		gesture.maxDX = Math.max(
			gesture.maxDX,
			Math.abs(touch.clientX - gesture.x),
		);
		gesture.maxDY = Math.max(
			gesture.maxDY,
			Math.abs(touch.clientY - gesture.y),
		);
	},
	{ capture: true, passive: true },
);
const endGesture = (event: TouchEvent) => {
	touchesDown = event.touches.length;
	lastTouchEndAt = Date.now();
	if (!gesture) {
		return;
	}
	const ended = gesture;
	gesture = null;
	record("gesture", {
		ending: event.type,
		ms: Date.now() - ended.startedAt,
		from: [ended.x, ended.y],
		dx: Math.round(ended.lastX - ended.x),
		dy: Math.round(ended.lastY - ended.y),
		maxDX: Math.round(ended.maxDX),
		maxDY: Math.round(ended.maxDY),
		width: innerWidth,
		pathAtStart: ended.path,
		start: ended.start,
		transcriptAtStart: ended.transcript,
		transcriptScrollsInGesture,
		overscrollX: overscrollX(),
		activation: activation(),
	});
};
window.addEventListener("touchend", endGesture, {
	capture: true,
	passive: true,
});
window.addEventListener("touchcancel", endGesture, {
	capture: true,
	passive: true,
});

let lastSnapshot = "";
setInterval(() => {
	const current = snapshot();
	const serialized = safeJSON(current);
	if (serialized !== lastSnapshot) {
		lastSnapshot = serialized;
		record("change", current);
	}
}, 500);
setInterval(() => record("alive"), 5000);

const readServiceWorkerEntries = async (): Promise<unknown> => {
	const read = async () => {
		const cache = await caches.open("swipe-diag");
		const response = await cache.match("/__swipe-diag");
		return response ? await response.json() : [];
	};
	const timeout = new Promise<string>((resolve) =>
		setTimeout(() => resolve("cache read timed out"), 3000),
	);
	try {
		return await Promise.race([read(), timeout]);
	} catch (error) {
		return String(error);
	}
};

const openPanel = async () => {
	const text = JSON.stringify({
		version: diagVersion,
		page: readEntries(),
		serviceWorker: await readServiceWorkerEntries(),
	});
	const panel = document.createElement("div");
	Object.assign(panel.style, {
		position: "fixed",
		inset: "0",
		zIndex: "2147483647",
		background: "#fff",
		color: "#000",
		display: "flex",
		flexDirection: "column",
		gap: "8px",
		padding: "calc(env(safe-area-inset-top) + 8px) 8px 8px",
		font: "14px system-ui",
	});
	const info = document.createElement("div");
	info.textContent = `${text.length} characters. Copy, paste into the chat, then Clear.`;
	const versions = document.createElement("div");
	const showVersions = () => {
		versions.textContent = `Page ${diagVersion}, worker ${navigator.serviceWorker?.controller?.scriptURL ?? "none"}. This page started at ${launchPath} (history length ${launchHistoryLength}). Transcript overscroll-x: ${overscrollX()}.`;
	};
	showVersions();
	const textarea = document.createElement("textarea");
	textarea.value = text;
	textarea.readOnly = true;
	Object.assign(textarea.style, { flex: "1", font: "11px monospace" });
	const row = document.createElement("div");
	Object.assign(row.style, { display: "flex", gap: "8px" });
	const action = (
		label: string,
		onClick: (self: HTMLButtonElement) => void,
	) => {
		const element = document.createElement("button");
		element.textContent = label;
		Object.assign(element.style, {
			flex: "1",
			padding: "12px",
			font: "16px system-ui",
		});
		element.addEventListener("click", () => onClick(element));
		row.appendChild(element);
	};
	action("Copy", (self) => {
		navigator.clipboard.writeText(text).then(
			() => {
				self.textContent = "Copied";
			},
			() => {
				textarea.select();
				self.textContent = "Copy failed, text selected";
			},
		);
	});
	action("Clear", (self) => {
		localStorage.removeItem(storageKey);
		void caches
			.open("swipe-diag")
			.then((cache) => cache.delete("/__swipe-diag"));
		self.textContent = "Cleared";
	});
	action("Close", () => panel.remove());
	const overscrollRow = document.createElement("div");
	Object.assign(overscrollRow.style, { display: "flex", gap: "8px" });
	for (const [label, value] of [
		["Overscroll X: app default", null],
		["Overscroll X: auto", "auto"],
	] as const) {
		const element = document.createElement("button");
		element.textContent = label;
		Object.assign(element.style, {
			flex: "1",
			padding: "12px 4px",
			font: "14px system-ui",
		});
		element.addEventListener("click", () => {
			if (value) {
				localStorage.setItem(overscrollXKey, value);
			} else {
				localStorage.removeItem(overscrollXKey);
			}
			applyOverscrollX();
			record("overscrollX", { value: overscrollX() });
			showVersions();
		});
		overscrollRow.appendChild(element);
	}
	panel.append(versions, overscrollRow, info, row, textarea);
	document.body.appendChild(panel);
};

const mountButton = () => {
	// The label never changes: replacing the text node under a pending tap
	// makes WebKit drop the click.
	const button = document.createElement("button");
	button.textContent = "Diag";
	Object.assign(button.style, {
		position: "fixed",
		right: "0",
		top: "45%",
		zIndex: "2147483646",
		padding: "6px 4px",
		font: "11px system-ui",
		background: "#b91c1c",
		color: "#fff",
		border: "0",
		borderRadius: "6px 0 0 6px",
		opacity: "0.85",
	});
	button.addEventListener("click", () => void openPanel());
	document.body.appendChild(button);
};

if (document.body) {
	mountButton();
} else {
	document.addEventListener("DOMContentLoaded", mountButton);
}

export {};
