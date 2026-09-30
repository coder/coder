import { waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatAutomation } from "#/testHelpers/chatEntities";
import { createDeferred } from "#/testHelpers/deferred";
import { renderHookWithAuth } from "#/testHelpers/hooks";
import { server } from "#/testHelpers/server";
import { useChatAutomationNames } from "./useChatAutomationNames";

const mockExperiments = (experiments: TypesGen.Experiment[]) =>
	server.use(
		http.get("/api/v2/experiments", () => HttpResponse.json(experiments)),
	);

describe("useChatAutomationNames", () => {
	it("reports loading until the automations list resolves, then maps IDs to names", async () => {
		mockExperiments(["chat-automations"]);
		const response = createDeferred<undefined>();
		server.use(
			http.get(
				"/api/experimental/organizations/:organizationId/chat-automations",
				async () => {
					await response.promise;
					return HttpResponse.json([MockChatAutomation]);
				},
			),
		);

		const { result } = await renderHookWithAuth(
			() => useChatAutomationNames("test-org-id", true),
			{},
		);

		await waitFor(() => expect(result.current.isLoading).toBe(true));
		expect(result.current.names.size).toBe(0);

		response.resolve(undefined);
		await waitFor(() => expect(result.current.isLoading).toBe(false));
		expect(result.current.names.get(MockChatAutomation.id)).toBe(
			MockChatAutomation.name,
		);
	});

	it("does not report loading when the chat-automations experiment is off", async () => {
		mockExperiments([]);

		const { result } = await renderHookWithAuth(
			() => useChatAutomationNames("test-org-id", true),
			{},
		);

		expect(result.current).toEqual({ names: new Map(), isLoading: false });
	});
});
