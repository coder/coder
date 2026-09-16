import { preloadHighlighter } from "@pierre/diffs";
import { setProjectAnnotations } from "@storybook/react-vite";
import { beforeAll, beforeEach } from "vitest";
import * as previewAnnotations from "./preview";

const annotations = setProjectAnnotations([previewAnnotations]);

beforeAll(annotations.beforeAll);

// Stories render diff viewers without the app's worker pool, so @pierre/diffs
// falls back to the shared in-page highlighter. Under StrictMode's dev-only
// remount, a cold highlighter loses the first render: the remounted instance
// re-hydrates the empty <pre> left behind and never schedules another render,
// leaving the diff blank. Warming the themes here makes the first render
// synchronous so re-hydration always finds content.
beforeAll(async () => {
	await preloadHighlighter({
		themes: ["github-dark-high-contrast", "github-light"],
		langs: [],
	});
});

// Radix DismissableLayer sets document.body.style.pointerEvents = "none" while
// a modal layer is active. When a story unmounts, the useEffect cleanup that
// restores body.pointerEvents can race with the next story's play function,
// causing false "pointer-events: none" failures on the first click.
beforeEach(() => {
	document.body.style.pointerEvents = "";
});
