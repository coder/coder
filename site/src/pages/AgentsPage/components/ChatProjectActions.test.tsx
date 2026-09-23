import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { describe, expect, it } from "vitest";
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

type WrapperProps = PropsWithChildren;

const Wrapper: FC<WrapperProps> = ({ children }) => {
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

const chatInProjectOrganization = {
	...MockChat,
	organization_id: MockChatProject.organization_id,
};

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
				<ChatProjectActions chat={chatInProjectOrganization} menu="dropdown" />
			</Wrapper>,
		);

		await user.click(screen.getByRole("menuitem", { name: "Move to project" }));
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

	it("does not offer projects owned by another user", async () => {
		const user = userEvent.setup();
		const otherUsersProject = {
			...MockChatProject,
			id: "other-users-project",
			owner_id: "other-user",
			name: "Other user's project",
		};
		server.use(
			http.get("/api/experimental/chats/projects", () =>
				HttpResponse.json([MockChatProject, otherUsersProject]),
			),
		);

		render(
			<Wrapper>
				<ChatProjectActions chat={chatInProjectOrganization} menu="dropdown" />
			</Wrapper>,
		);

		await user.click(screen.getByRole("menuitem", { name: "Move to project" }));
		await user.keyboard("{ArrowRight}");
		await screen.findByRole("menuitem", { name: MockChatProject.name });
		expect(
			screen.queryByRole("menuitem", { name: otherUsersProject.name }),
		).toBeNull();
	});

	it("clears the project with the nil UUID", async () => {
		const user = userEvent.setup();
		let requestBody: unknown;
		server.use(
			http.get("/api/experimental/chats/projects", () =>
				HttpResponse.json([MockChatProject]),
			),
			http.patch("*", async ({ request }) => {
				requestBody = await request.json();
				return new HttpResponse(null, { status: 204 });
			}),
		);

		render(
			<Wrapper>
				<ChatProjectActions chat={chatInProjectOrganization} menu="dropdown" />
			</Wrapper>,
		);

		await user.click(screen.getByRole("menuitem", { name: "Move to project" }));
		await user.keyboard("{ArrowRight}");
		const noProject = await screen.findByRole("menuitem", {
			name: "No project",
		});
		noProject.focus();
		await user.keyboard("{Enter}");

		await waitFor(() => {
			expect(requestBody).toEqual({
				project_id: "00000000-0000-0000-0000-000000000000",
			});
		});
	});

	it("retries a failed project list from the submenu", async () => {
		const user = userEvent.setup();
		let requestCount = 0;
		server.use(
			http.get("/api/experimental/chats/projects", () => {
				requestCount++;
				return requestCount === 1
					? HttpResponse.json(
							{ message: "Projects unavailable" },
							{ status: 500 },
						)
					: HttpResponse.json([MockChatProject]);
			}),
		);

		render(
			<Wrapper>
				<ChatProjectActions chat={chatInProjectOrganization} menu="dropdown" />
			</Wrapper>,
		);

		await user.click(screen.getByRole("menuitem", { name: "Move to project" }));
		await user.keyboard("{ArrowRight}");
		const retry = await screen.findByRole("menuitem", { name: "Retry" });
		retry.focus();
		await user.keyboard("{Enter}");

		await waitFor(() => expect(requestCount).toBe(2));
	});
});
