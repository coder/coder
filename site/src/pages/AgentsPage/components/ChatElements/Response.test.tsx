import { screen, waitFor, within } from "@testing-library/react";
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

const chip = { element: <button>chip</button>, text: "chip" };

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

	it("copies an inline node inside fenced code as its text", async () => {
		const user = userEvent.setup();
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue();

		render(
			<Response>
				{[
					"```\nopen ",
					{ element: <button>chip</button>, text: "main.go:L1" },
					"\n```",
				]}
			</Response>,
		);

		await user.click(await screen.findByRole("button", { name: "Copy code" }));

		expect(writeText).toHaveBeenCalledWith("open main.go:L1");
	});

	it("ends an autolink at an inline node that touches the URL", async () => {
		render(
			<Response>{["See https://example.com/docs", chip, " now"]}</Response>,
		);

		const link = await screen.findByRole("link");
		expect(link).toHaveAccessibleName("https://example.com/docs");
		expect(link).toHaveAttribute("href", "https://example.com/docs");
	});

	it("never resolves typed delimiters, including character references", async () => {
		render(<Response>{["\uE0000\uE001 &#xE000;0&#57345; ", chip]}</Response>);

		await screen.findByText(/\uFFFD0\uFFFD \uFFFD0\uFFFD/);
		expect(screen.getAllByRole("button", { name: "chip" })).toHaveLength(1);
	});

	describe("ordered list numbers", () => {
		const renderListNumbers = async (
			markdown: string,
			props: { streaming?: boolean; preserveLineBreaks?: boolean } = {},
		) => {
			render(<Response {...props}>{markdown}</Response>);
			const items = await screen.findAllByRole("listitem");
			return items.map((item) => item.getAttribute("value"));
		};

		it("keeps the numbers of a list that skips a step", async () => {
			const numbers = await renderListNumbers(
				"Steps:\n\n1. First\n3. Third\n4. Fourth\n5. Fifth",
			);
			expect(numbers).toEqual(["1", "3", "4", "5"]);
		});

		it("keeps a list's start without numbering items that count up from it", async () => {
			render(<Response>{"3. Third\n4. Fourth"}</Response>);
			const list = await screen.findByRole("list");
			expect(list).toHaveAttribute("start", "3");
			const items = within(list).getAllByRole("listitem");
			expect(items.map((item) => item.getAttribute("value"))).toEqual([
				null,
				null,
			]);
		});

		it("keeps the numbers of a list that starts after one and skips a step", async () => {
			const numbers = await renderListNumbers("3. Third\n5. Fifth");
			expect(numbers).toEqual(["3", "5"]);
		});

		it("numbers a list that repeats its first number automatically", async () => {
			const numbers = await renderListNumbers("1. One\n1. Two\n1. Three");
			expect(numbers).toEqual([null, null, null]);
		});

		it("keeps the numbers of a nested list on their own", async () => {
			const numbers = await renderListNumbers(
				"1. Outer\n   1. Inner\n   3. Inner skipped\n2. Outer next",
			);
			expect(numbers).toEqual([null, "1", "3", null]);
		});

		it("keeps the numbers of a list with parenthesis markers", async () => {
			const numbers = await renderListNumbers("1) One\n4) Four");
			expect(numbers).toEqual(["1", "4"]);
		});

		it("keeps the numbers of a loose list", async () => {
			const numbers = await renderListNumbers("1. One\n\n3. Three\n\n7. Seven");
			expect(numbers).toEqual(["1", "3", "7"]);
		});

		it("keeps the numbers of a list in a block quote", async () => {
			const numbers = await renderListNumbers("> 1. One\n> 3. Three");
			expect(numbers).toEqual(["1", "3"]);
		});

		it("keeps the numbers of a list while streaming", async () => {
			const numbers = await renderListNumbers(
				"Some **bold** text first.\n\n1. First\n3. Third\n4. Fourth",
				{ streaming: true },
			);
			expect(numbers).toEqual(["1", "3", "4"]);
		});

		it("keeps the numbers of a list with preserved line breaks", async () => {
			const numbers = await renderListNumbers("1. First\n3. Third", {
				preserveLineBreaks: true,
			});
			expect(numbers).toEqual(["1", "3"]);
		});
	});

	// Strikethrough consumes its tildes, so whether they survive in the
	// rendered text tells the two apart.
	describe("strikethrough", () => {
		it("leaves single tildes as written", async () => {
			const text = "Each step takes ~5 min, so 10~15 min total.";
			render(<Response>{text}</Response>);
			await waitFor(() => expect(document.body).toHaveTextContent(text));
		});

		it("strikes text between double tildes", async () => {
			render(<Response>Use ~~the old flag~~ the new one.</Response>);
			await waitFor(() =>
				expect(document.body).toHaveTextContent(
					"Use the old flag the new one.",
				),
			);
		});
	});
});
