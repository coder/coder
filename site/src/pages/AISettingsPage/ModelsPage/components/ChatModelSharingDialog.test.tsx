import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type PathParams } from "msw";
import { QueryClientProvider } from "react-query";
import { describe, expect, it, vi } from "vitest";
import type {
	ChatModelACL,
	UpdateChatModelACLRequest,
} from "#/api/typesGenerated";
import {
	MockChatModelACLAvailable,
	MockGroup,
	MockOrganization,
	MockUserOwner,
} from "#/testHelpers/entities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { ChatModelSharingDialog } from "./ChatModelSharingDialog";

const aclPath = "/api/v2/organizations/:organization/chats/models/:model/acl";

describe("ChatModelSharingDialog", () => {
	it("saves only the changed entries", async () => {
		const acl: ChatModelACL = {
			users: [],
			groups: [{ ...MockGroup, role: "read" }],
		};
		const updates: unknown[] = [];
		server.use(
			http.get(aclPath, () => HttpResponse.json(acl)),
			http.get(`${aclPath}/available`, () =>
				HttpResponse.json(MockChatModelACLAvailable),
			),
			http.patch<PathParams, UpdateChatModelACLRequest>(
				aclPath,
				async ({ params, request }) => {
					updates.push({ ...params, body: await request.json() });
					return new HttpResponse(null, { status: 204 });
				},
			),
		);
		const onOpenChange = vi.fn();
		const user = userEvent.setup();
		renderComponent(
			<QueryClientProvider client={createTestQueryClient()}>
				<ChatModelSharingDialog
					open
					onOpenChange={onOpenChange}
					organizationId={MockOrganization.id}
					modelId="model-1"
					modelName="GPT-5"
				/>
			</QueryClientProvider>,
		);

		await user.click(
			await screen.findByRole("button", {
				name: `Remove ${MockGroup.display_name}`,
			}),
		);
		await user.click(
			screen.getByRole("button", { name: "Search for user or group" }),
		);
		await user.type(
			screen.getByPlaceholderText("Search for user or group"),
			MockUserOwner.email,
		);
		await user.click(
			await screen.findByRole("option", {
				name: new RegExp(MockUserOwner.email, "i"),
			}),
		);
		await user.click(screen.getByRole("button", { name: "Add member" }));
		await user.click(screen.getByRole("button", { name: "Save permissions" }));

		await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
		expect(updates).toStrictEqual([
			{
				organization: MockOrganization.id,
				model: "model-1",
				body: {
					user_roles: { [MockUserOwner.id]: "read" },
					group_roles: { [MockGroup.id]: "" },
				},
			},
		]);
	});
});
