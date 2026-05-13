import { setProjectAnnotations } from "@storybook/react-vite";
import { screenshot } from "@storycap-testrun/browser";
import { afterEach, beforeAll, beforeEach } from "vitest";
import { page } from "vitest/browser";
import * as previewAnnotations from "./preview";

const annotations = setProjectAnnotations([previewAnnotations]);
const isVisualRegression = process.env.VISUAL_REGRESSION === "true";

beforeAll(annotations.beforeAll);

if (isVisualRegression) {
	beforeEach(async () => {
		await page.viewport(1280, 720);
	});

	afterEach(async (context) => {
		await screenshot(page, context, {
			flakiness: {
				metrics: { enabled: false },
				retake: { enabled: false },
			},
			fullPage: true,
			scale: "css",
		});
	});
}
