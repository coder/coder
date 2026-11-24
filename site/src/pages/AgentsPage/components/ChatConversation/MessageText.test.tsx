import { describe, expect, it, vi } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { ResponseBlock } from "./MessageText";

// The real viewer paints a code block's text in a shadow root, with styles
// jsdom's stylesheet parser cannot read, so this stand-in paints it there
// without them.
vi.mock("@pierre/diffs/react", () => ({
	File: ({ file }: { file: { contents: string } }) => (
		<div
			ref={(node) => {
				if (node && !node.shadowRoot) {
					node.attachShadow({ mode: "open" }).textContent = file.contents;
				}
			}}
		/>
	),
}));

describe("ResponseBlock", () => {
	it.each([
		["text", "The workspace looks healthy."],
		["only a code block", "```ts\nconst marker = 1;\n```"],
	])("reports shown text for a reply of %s", (_name, text) => {
		const onTextShownChange = vi.fn();
		renderComponent(
			<ResponseBlock
				text={text}
				isStreaming={false}
				streamKey="live-assistant"
				onTextShownChange={onTextShownChange}
			/>,
		);

		expect(onTextShownChange).toHaveBeenLastCalledWith("live-assistant", true);
	});
});
