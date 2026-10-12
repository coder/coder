import { screen } from "@testing-library/react";
import { QueryClientProvider } from "react-query";
import { describe, expect, it } from "vitest";
import type { ChatAutomation } from "#/api/typesGenerated";
import {
	MockChatAutomation,
	MockWebhookChatAutomation,
} from "#/testHelpers/chatEntities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { AutomationSummary } from "./AutomationSummary";
import { WEBHOOK_ENDPOINT_HINT } from "./AutomationWebhookFields";

const newChatAutomation: ChatAutomation = {
	...MockChatAutomation,
	target_mode: "new_chat",
	target_chat_id: undefined,
	target_chat: undefined,
	when_busy: undefined,
};

describe("AutomationSummary", () => {
	it("names the run mode as the list does, under one Runs in heading", () => {
		renderComponent(
			<QueryClientProvider client={createTestQueryClient()}>
				<AutomationSummary
					automation={newChatAutomation}
					origin="https://coder.example.com"
				/>
			</QueryClientProvider>,
		);

		expect(
			screen.getAllByText("Runs in").map((element) => element.tagName),
		).toEqual(["H3"]);
		expect(screen.getByText("New chat each run").tagName).toBe("DD");
	});

	it("labels the webhook endpoint and its hint as the editor does", () => {
		renderComponent(
			<QueryClientProvider client={createTestQueryClient()}>
				<AutomationSummary
					automation={MockWebhookChatAutomation}
					origin="https://coder.example.com"
				/>
			</QueryClientProvider>,
		);

		expect(screen.getByText("Endpoint").tagName).toBe("DT");
		expect(screen.queryByText("Publish endpoint")).toBeNull();
		screen.getByText(WEBHOOK_ENDPOINT_HINT);
	});
});
