import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it } from "vitest";
import {
	DropdownMenu,
	DropdownMenuContent,
} from "#/components/DropdownMenu/DropdownMenu";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockChatProject,
	MockDefaultOrganization,
	MockEntitlements,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { ChatProjectActions } from "./ChatProjectActions";

const Wrapper: FC<PropsWithChildren> = ({ children }) => {
	const queryClient = createTestQueryClient();
	return (
		<QueryClientProvider client={queryClient}>
			<DashboardContext.Provider
				value={{
					entitlements: MockEntitlements,
					experiments: ["chat-projects"],
					appearance: MockAppearanceConfig,
					buildInfo: MockBuildInfo,
					organizations: [MockDefaultOrganization],
					showOrganizations: false,
					canViewOrganizationSettings: false,
				}}
			>
				<DropdownMenu open>
					<DropdownMenuContent>{children}</DropdownMenuContent>
				</DropdownMenu>
			</DashboardContext.Provider>
		</QueryClientProvider>
	);
};

afterEach(() => server.resetHandlers());

describe("ChatProjectActions", () => {
	it("assigns the selected project with a chat PATCH request", async () => {
		const user = userEvent.setup();
		let requestBody: unknown;
		let requestURL: string | undefined;
		server.use(
			http.get("/api/experimental/chats/projects", () =>
				HttpResponse.json([MockChatProject]),
			),
			http.patch("*", async ({ request }) => {
				requestURL = request.url;
				requestBody = await request.json();
				return new HttpResponse(null, { status: 204 });
			}),
		);

		render(
			<Wrapper>
				<ChatProjectActions chat={MockChat} menu="dropdown" />
			</Wrapper>,
		);

		const moveToProject = screen.getByRole("menuitem", {
			name: "Move to project",
		});
		await user.click(moveToProject);
		await user.keyboard("{ArrowRight}");
		const projectItem = await screen.findByRole("menuitem", {
			name: MockChatProject.name,
		});
		projectItem.focus();
		await user.keyboard("{Enter}");

		await waitFor(() => {
			expect(requestURL).toContain(`/api/v2/chats/${MockChat.id}`);
			expect(requestBody).toEqual({ project_id: MockChatProject.id });
		});
	});
});
