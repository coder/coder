import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { render } from "#/testHelpers/renderHelpers";
import {
	escapeInlineNodeDelimiters,
	inlineNodePlaceholder,
} from "./inlineNodes";
import { Response } from "./Response";

// jsdom's stylesheet parser throws on the real FileViewer's shadow-DOM styles.
vi.mock("@pierre/diffs/react", () => ({
	File: ({ file }: { file: { contents: string } }) => (
		<pre>{file.contents}</pre>
	),
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

	it("copies an inline node inside fenced code as its fallback text", async () => {
		const user = userEvent.setup();
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue();

		render(
			<Response
				inlineNodes={[{ element: <button>chip</button>, text: "main.go:L1" }]}
			>
				{`\`\`\`\nopen ${inlineNodePlaceholder(0)}\n\`\`\``}
			</Response>,
		);

		await user.click(await screen.findByRole("button", { name: "Copy code" }));

		expect(writeText).toHaveBeenCalledWith("open main.go:L1");
	});

	it("ends an autolink at an inline node that touches the URL", async () => {
		render(
			<Response
				inlineNodes={[{ element: <button>chip</button>, text: "chip" }]}
			>
				{`See https://example.com/docs${inlineNodePlaceholder(0)} now`}
			</Response>,
		);

		const link = await screen.findByRole("link");
		expect(link).toHaveAccessibleName("https://example.com/docs");
		expect(link).toHaveAttribute("href", "https://example.com/docs");
	});

	it("never resolves typed delimiters, including character references", async () => {
		render(
			<Response
				inlineNodes={[{ element: <button>chip</button>, text: "chip" }]}
			>
				{escapeInlineNodeDelimiters("\uE0000\uE001 &#xE000;0&#57345;")}
			</Response>,
		);

		await screen.findByText("\uFFFD0\uFFFD \uFFFD0\uFFFD");
		expect(screen.queryByRole("button", { name: "chip" })).toBeNull();
	});
});
