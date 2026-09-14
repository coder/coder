import { formatAnnotations } from "./formatAnnotations";
import type { AnnotationSubmission } from "./protocol";

const submission: AnnotationSubmission = {
	page: {
		url: "http://localhost:3000/settings",
		title: "Settings",
		viewport: { width: 1280, height: 720 },
	},
	annotations: [
		{
			id: "1",
			comment: "Make this button primary",
			selectedText: "Save",
			element: {
				tag: "button",
				selector: '[data-testid="save"]',
				testId: "save",
				classes: ["btn", "secondary"],
				role: "button",
				ariaLabel: "Save changes",
				text: "Save",
				openingTag: '<button class="btn secondary">',
				rect: { x: 860, y: 300, width: 120, height: 40 },
				reactComponents: ["SaveButton", "SettingsForm"],
				sourceLocation: "src/SettingsForm.tsx:42",
			},
		},
		{
			id: "2",
			comment: "",
			element: {
				tag: "p",
				selector: "main > p",
				classes: [],
				openingTag: "<p>",
				rect: { x: 0, y: 0, width: 10, height: 10 },
			},
		},
	],
};

describe("formatAnnotations", () => {
	it("renders a header and one section per annotation", () => {
		const output = formatAnnotations(submission);
		expect(output).toMatchInlineSnapshot(`
			"# UI annotations

			Page: http://localhost:3000/settings (Settings)
			Viewport: 1280x720
			Count: 2

			## 1. Make this button primary

			- Element: \`[data-testid="save"]\` (\`<button>\`)
			- Test id: \`save\`
			- Accessibility: role="button", aria-label="Save changes"
			- Text: "Save"
			- Selected text: "Save"
			- React: SaveButton < SettingsForm
			- Source: \`src/SettingsForm.tsx:42\`
			- Classes: \`btn secondary\`
			- Position: 120x40 at (860, 300)
			- Tag: \`<button class="btn secondary">\`

			## 2. (no comment)

			- Element: \`main > p\` (\`<p>\`)
			- Position: 10x10 at (0, 0)
			- Tag: \`<p>\`
			"
		`);
	});
});
