import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { render } from "#/testHelpers/renderHelpers";
import { Response } from "./Response";

// jsdom's stylesheet parser throws on the real FileViewer's shadow-DOM styles.
vi.mock("@pierre/diffs/react", () => ({
	File: ({ file }: { file: { contents: string } }) => (
		<pre>{file.contents}</pre>
	),
}));

// jsdom cannot lay out a real Mermaid diagram.
vi.mock("mermaid", () => ({
	default: {
		initialize: vi.fn(),
		render: vi.fn(async () => ({ svg: "<svg></svg>" })),
	},
}));

const sampleFileCode = `package auth

import "errors"

func ValidateToken(token string) error {
	if token == "" {
		return errors.New("token is empty")
	}
	return nil
}`;

describe("Response", () => {
	it("copies a multi-line fenced code block's raw text, without the fence markers or trailing newline", async () => {
		const user = userEvent.setup();
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue();

		render(<Response>{`\`\`\`go\n${sampleFileCode}\n\`\`\`\n`}</Response>);

		await user.click(await screen.findByRole("button", { name: "Copy code" }));

		expect(writeText).toHaveBeenCalledWith(sampleFileCode);
	});

	it("returns focus to a Mermaid diagram once its preview closes", async () => {
		const user = userEvent.setup();
		render(<Response>{"```mermaid\nflowchart TD\n  A --> B\n```\n"}</Response>);
		const diagram = await screen.findByRole("button", {
			name: "View diagram full size",
		});

		await user.click(diagram);
		await screen.findByRole("dialog", { name: "Diagram preview" });
		await user.keyboard("{Escape}");

		await waitFor(() => expect(diagram).toHaveFocus());
	});
});
