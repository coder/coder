import { setProjectAnnotations } from "@storybook/react-vite";
import { beforeAll, beforeEach } from "vitest";
import * as previewAnnotations from "./preview";

const annotations = setProjectAnnotations([previewAnnotations]);

beforeAll(annotations.beforeAll);

// Radix DismissableLayer sets document.body.style.pointerEvents = "none" while
// a modal layer is active. When a story unmounts, the useEffect cleanup that
// restores body.pointerEvents can race with the next story's play function,
// causing false "pointer-events: none" failures on the first click.
beforeEach(() => {
	document.body.style.pointerEvents = "";
});
