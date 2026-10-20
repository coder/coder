/** Every column the sessions table can show, in their canonical order. */
export const sessionColumnIds = [
	"last_prompt",
	"user",
	"provider",
	"client",
	"model",
	"tokens",
	"cost",
	"network",
	"threads",
	"last_active",
] as const;

/** Identifies a configurable sessions table column. */
export type SessionColumnId = (typeof sessionColumnIds)[number];

type SessionColumn = {
	label: string;
	/** Pixels the column needs to fit; all but the last prompt render at it. */
	width: number;
};

/** Display label and width for each column. */
export const sessionColumns: Record<SessionColumnId, SessionColumn> = {
	last_prompt: { label: "Last prompt", width: 320 },
	user: { label: "User", width: 176 },
	provider: { label: "Provider", width: 160 },
	client: { label: "Client", width: 160 },
	model: { label: "Model", width: 192 },
	tokens: { label: "In/Out tokens", width: 168 },
	cost: { label: "Cost", width: 112 },
	network: { label: "Network", width: 152 },
	threads: { label: "Threads", width: 112 },
	last_active: { label: "Last active", width: 152 },
};

/** Width of the row chevron, which is always shown and cannot be moved. */
export const chevronColumnWidth = 48;

/** Shown columns in display order plus hidden ones; together they hold every column once. */
export type SessionColumnSettings = {
	shown: readonly SessionColumnId[];
	notShown: readonly SessionColumnId[];
};

/** Identifies a built-in column view. */
export type SessionColumnViewId = "general" | "security" | "cost";

type SessionColumnView = {
	id: SessionColumnViewId;
	label: string;
	settings: SessionColumnSettings;
};

/** Built-in views the user can apply in one click. */
export const sessionColumnViews: readonly SessionColumnView[] = [
	{
		id: "general",
		label: "General",
		settings: {
			shown: ["last_prompt", "user", "client", "tokens", "cost", "last_active"],
			notShown: ["provider", "model", "threads", "network"],
		},
	},
	{
		id: "security",
		label: "Security",
		settings: {
			shown: ["last_prompt", "user", "network", "threads", "last_active"],
			notShown: ["provider", "model", "tokens", "client", "cost"],
		},
	},
	{
		id: "cost",
		label: "Cost",
		settings: {
			shown: ["last_prompt", "user", "client", "cost", "tokens", "threads"],
			notShown: ["last_active", "network", "provider", "model"],
		},
	},
];

/** Settings for a user who has not customized the columns. */
export const defaultSessionColumnSettings = sessionColumnViews[0].settings;

/** Returns the view matching the shown columns, or undefined once customized. */
export const getActiveView = (
	settings: SessionColumnSettings,
): SessionColumnView | undefined =>
	sessionColumnViews.find(
		(view) =>
			view.settings.shown.length === settings.shown.length &&
			view.settings.shown.every((id, i) => settings.shown[i] === id),
	);

/** Shows a column last or hides it first in not shown; the last shown column stays. */
export const toggleColumn = (
	settings: SessionColumnSettings,
	id: SessionColumnId,
): SessionColumnSettings => {
	if (settings.shown.includes(id)) {
		if (settings.shown.length === 1) {
			return settings;
		}
		return {
			shown: settings.shown.filter((c) => c !== id),
			notShown: [id, ...settings.notShown],
		};
	}
	return {
		shown: [...settings.shown, id],
		notShown: settings.notShown.filter((c) => c !== id),
	};
};

/** Reinserts a shown column at `toIndex`, shifting the columns in between. */
export const moveShownColumn = (
	settings: SessionColumnSettings,
	id: SessionColumnId,
	toIndex: number,
): SessionColumnSettings => {
	const fromIndex = settings.shown.indexOf(id);
	if (fromIndex === -1 || fromIndex === toIndex) {
		return settings;
	}
	const shown = settings.shown.filter((c) => c !== id);
	shown.splice(toIndex, 0, id);
	return { ...settings, shown };
};

const isSessionColumnId = (value: unknown): value is SessionColumnId =>
	sessionColumnIds.some((id) => id === value);

const parseIds = (value: unknown): SessionColumnId[] =>
	Array.isArray(value) ? value.filter(isSessionColumnId) : [];

/** Restores saved settings; unknown columns drop and new ones start hidden. */
export const parseSessionColumnSettings = (
	raw: string | null,
): SessionColumnSettings => {
	if (raw === null) {
		return defaultSessionColumnSettings;
	}
	let parsed: unknown;
	try {
		parsed = JSON.parse(raw);
	} catch {
		return defaultSessionColumnSettings;
	}
	if (typeof parsed !== "object" || parsed === null) {
		return defaultSessionColumnSettings;
	}
	const shown = [
		...new Set(parseIds("shown" in parsed ? parsed.shown : undefined)),
	];
	if (shown.length === 0) {
		return defaultSessionColumnSettings;
	}
	const notShown = [
		...new Set([
			...parseIds("notShown" in parsed ? parsed.notShown : undefined),
			...sessionColumnIds,
		]),
	].filter((id) => !shown.includes(id));
	return { shown, notShown };
};

/** Counts leading shown columns that fit beside the chevron, keeping at least one. */
export const countFittingColumns = (
	shown: readonly SessionColumnId[],
	availableWidth: number,
): number => {
	let used = chevronColumnWidth;
	for (const [i, id] of shown.entries()) {
		used += sessionColumns[id].width;
		if (used > availableWidth) {
			return Math.max(i, 1);
		}
	}
	return shown.length;
};
