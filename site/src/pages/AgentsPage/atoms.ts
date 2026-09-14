import {
	atomWithStorage,
	createJSONStorage,
	unstable_withStorageValidator as withStorageValidator,
} from "jotai/utils";
import type { DiffStyle } from "./components/DiffViewer/DiffViewer";

const storage = createJSONStorage<unknown>(() => localStorage);

const booleanStorage = withStorageValidator(
	(value): value is boolean => typeof value === "boolean",
)(storage);

const numberStorage = withStorageValidator(
	(value): value is number => typeof value === "number",
)(storage);

/** Whether the chat layout uses the full available width. */
export const chatFullWidthAtom = atomWithStorage(
	"agents.chat-full-width",
	false,
	booleanStorage,
	{ getOnInit: true },
);

/** Whether a sound plays when a background chat finishes. */
export const chimeEnabledAtom = atomWithStorage(
	"agents.chime-on-completion",
	false,
	booleanStorage,
	{ getOnInit: true },
);

/** Preferred layout for rendered Git diffs. */
export const diffStyleAtom = atomWithStorage<DiffStyle>(
	"agents.diff-view-style",
	"unified",
	withStorageValidator(
		(value): value is DiffStyle => value === "split" || value === "unified",
	)(storage),
	{ getOnInit: true },
);

/** Whether the chat right panel is open. */
export const rightPanelOpenAtom = atomWithStorage(
	"agents.right-panel-open",
	false,
	booleanStorage,
	{ getOnInit: true },
);

/** Width of the chat right panel in pixels; consumers clamp it. */
export const rightPanelWidthAtom = atomWithStorage(
	"agents.right-panel-width",
	480,
	numberStorage,
	{ getOnInit: true },
);

/** Width of the chats sidebar in pixels; consumers clamp it. */
export const leftSidebarWidthAtom = atomWithStorage(
	"agents.left-sidebar-width",
	320,
	numberStorage,
	{ getOnInit: true },
);

/** Model configuration selected for the most recent chat. */
export const lastModelConfigIDAtom = atomWithStorage<string | null>(
	"agents.last-model-config-id",
	null,
	withStorageValidator(
		(value): value is string | null =>
			typeof value === "string" || value === null,
	)(storage),
	{ getOnInit: true },
);
