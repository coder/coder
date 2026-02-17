import { renderComponent } from "testHelpers/renderHelpers";
import { parsePatchFiles } from "@pierre/diffs";
import { screen } from "@testing-library/react";
import { useQuery } from "react-query";
import type { Mock } from "vitest";
import { FilesChangedPanel } from "./FilesChangedPanel";

vi.mock("react-query", async () => {
	const actual =
		await vi.importActual<typeof import("react-query")>("react-query");
	return {
		...actual,
		useQuery: vi.fn(),
	};
});

vi.mock("@pierre/diffs", async () => {
	const actual =
		await vi.importActual<typeof import("@pierre/diffs")>("@pierre/diffs");
	return {
		...actual,
		parsePatchFiles: vi.fn(),
	};
});

const mockUseQuery = useQuery as unknown as Mock;
const mockParsePatchFiles = parsePatchFiles as unknown as Mock;

const makeQueryResult = (overrides: Record<string, unknown> = {}) => ({
	data: undefined,
	error: null,
	isLoading: false,
	isError: false,
	...overrides,
});

describe(FilesChangedPanel.name, () => {
	beforeEach(() => {
		mockUseQuery.mockReset();
		mockParsePatchFiles.mockReset();
		mockUseQuery.mockImplementation(() => makeQueryResult());
	});

	it("shows empty state when there is no diff content", () => {
		mockUseQuery
			.mockReturnValueOnce(makeQueryResult({ data: { url: undefined } }))
			.mockReturnValueOnce(makeQueryResult({ data: { diff: "" } }));

		renderComponent(<FilesChangedPanel chatId="chat-no-diff" />);

		expect(screen.getByText("No file changes to display.")).toBeInTheDocument();
		expect(mockParsePatchFiles).not.toHaveBeenCalled();
	});

	it("falls back to empty state when diff parsing fails", () => {
		mockUseQuery
			.mockReturnValueOnce(
				makeQueryResult({
					data: { url: "https://github.com/coder/coder/pull/123" },
				}),
			)
			.mockReturnValueOnce(
				makeQueryResult({
					data: { diff: "not-a-valid-unified-diff" },
				}),
			);
		mockParsePatchFiles.mockImplementation(() => {
			throw new Error("malformed diff");
		});

		expect(() => {
			renderComponent(<FilesChangedPanel chatId="chat-bad-diff" />);
		}).not.toThrow();
		expect(screen.getByText("No file changes to display.")).toBeInTheDocument();
		expect(mockParsePatchFiles).toHaveBeenCalledWith(
			"not-a-valid-unified-diff",
		);
	});
});
