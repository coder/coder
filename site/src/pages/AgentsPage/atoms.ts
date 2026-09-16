import {
	booleanCodec,
	defineStorageKey,
	integerCodec,
	stringCodec,
	stringLiteralCodec,
} from "#/storage";
import {
	LEFT_SIDEBAR_DEFAULT_WIDTH,
	LEFT_SIDEBAR_MAX_WIDTH,
	LEFT_SIDEBAR_MIN_WIDTH,
} from "./components/ChatsSidebar/sidebarWidth";
import type { DiffStyle } from "./components/DiffViewer/DiffViewer";

export const chatFullWidthAtom = defineStorageKey({
	key: "agents.chat-full-width",
	codec: booleanCodec,
	defaultValue: false,
});
export const chimeEnabledAtom = defineStorageKey({
	key: "agents.chime-on-completion",
	codec: booleanCodec,
	defaultValue: false,
});
export const diffStyleAtom = defineStorageKey<DiffStyle>({
	key: "agents.diff-view-style",
	codec: stringLiteralCodec({ oneOf: ["split", "unified"] }),
	defaultValue: "unified",
});
export const rightPanelOpenAtom = defineStorageKey({
	key: "agents.right-panel-open",
	codec: booleanCodec,
	defaultValue: false,
});
// Browser zoom can produce fractional drag widths; preserve them on upgrade.
export const rightPanelWidthAtom = defineStorageKey<number | null>({
	key: "agents.right-panel-width",
	codec: {
		decode: (raw: string) =>
			Number.isFinite(Number(raw)) ? Math.round(Number(raw)) : undefined,
		encode: (value: number) => String(value),
	},
	defaultValue: null,
});
export const leftSidebarWidthAtom = defineStorageKey({
	key: "agents.left-sidebar-width",
	codec: {
		encode: integerCodec.encode,
		decode: (raw) => {
			const width = integerCodec.decode(raw);
			return width !== undefined &&
				width >= LEFT_SIDEBAR_MIN_WIDTH &&
				width <= LEFT_SIDEBAR_MAX_WIDTH
				? width
				: undefined;
		},
	},
	defaultValue: LEFT_SIDEBAR_DEFAULT_WIDTH,
});
export const lastModelConfigIDAtom = defineStorageKey<string | null>({
	key: "agents.last-model-config-id",
	codec: stringCodec,
	defaultValue: null,
});
