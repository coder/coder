import { screen } from "@testing-library/react";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockUserPreferenceSettings } from "#/testHelpers/entities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { BlockList } from "./MessageBlocks";

afterEach(() => {
	vi.restoreAllMocks();
});

describe("BlockList", () => {
	it("keeps the default thinking title while a heading streams in", async () => {
		vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
			MockUserPreferenceSettings,
		);
		renderComponent(
			<QueryClientProvider client={createTestQueryClient()}>
				<BlockList
					blocks={[{ type: "thinking", text: "**Checking the co" }]}
					tools={[]}
					keyPrefix="streaming-heading"
					isStreaming
				/>
			</QueryClientProvider>,
		);

		// Wait until smoothing reveals the whole text in the body.
		await screen.findByText("Checking the co");
		expect(screen.getByRole("button", { name: "Thinking" })).toBeVisible();
	});
});
