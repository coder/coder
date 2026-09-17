import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, describe, expect, it } from "vitest";
import type { Chat } from "#/api/typesGenerated";
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
import AgentProjectPage from "./AgentProjectPage";

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
				<MemoryRouter
					initialEntries={[`/agents/projects/${MockChatProject.id}`]}
				>
					<Routes>
						<Route path="/agents/projects/:projectId" element={children} />
					</Routes>
				</MemoryRouter>
			</DashboardContext.Provider>
		</QueryClientProvider>
	);
};

const projectHandlers = (
	canUpdate: boolean,
	onChecked?: (checks: Record<string, unknown>) => void,
) => [
	http.get(`/api/experimental/chats/projects/${MockChatProject.id}`, () =>
		HttpResponse.json(MockChatProject),
	),
	http.get("/api/v2/chats", () => HttpResponse.json([])),
	http.post("/api/v2/authcheck", async ({ request }) => {
		const { checks } = (await request.json()) as {
			checks: Record<string, unknown>;
		};
		onChecked?.(checks);
		return HttpResponse.json(
			Object.fromEntries(Object.keys(checks).map((key) => [key, canUpdate])),
		);
	}),
];

const expectedPermissionChecks = expect.arrayContaining([
	{
		object: {
			resource_type: "chat_project",
			organization_id: MockChatProject.organization_id,
			owner_id: MockChatProject.created_by,
		},
		action: "update",
	},
	{
		object: {
			resource_type: "chat_project",
			organization_id: MockChatProject.organization_id,
			owner_id: MockChatProject.created_by,
		},
		action: "delete",
	},
]);

afterEach(() => server.resetHandlers());

describe("AgentProjectPage", () => {
	it("edits the project when the user may update it", async () => {
		const user = userEvent.setup();
		let authChecks: Record<string, unknown> | undefined;
		let patchBody: unknown;
		server.use(
			...projectHandlers(true, (checks) => {
				authChecks = checks;
			}),
			http.patch(
				`/api/experimental/chats/projects/${MockChatProject.id}`,
				async ({ request }) => {
					patchBody = await request.json();
					return HttpResponse.json({
						...MockChatProject,
						name: "Updated project",
					});
				},
			),
		);

		render(
			<Wrapper>
				<AgentProjectPage />
			</Wrapper>,
		);

		await waitFor(() => expect(authChecks).toBeDefined());
		expect(Object.values(authChecks ?? {})).toEqual(expectedPermissionChecks);
		await user.click(screen.getByRole("button", { name: "Edit" }));
		const nameInput = screen.getByLabelText("Name");
		expect(nameInput).toHaveValue(MockChatProject.name);
		await user.clear(nameInput);
		await user.type(nameInput, "Updated project");
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(patchBody).toEqual({
				name: "Updated project",
				description: MockChatProject.description,
			});
		});
	});

	it("hides Edit when the user may not update the project", async () => {
		let authChecks: Record<string, unknown> | undefined;
		server.use(
			...projectHandlers(false, (checks) => {
				authChecks = checks;
			}),
		);

		render(
			<Wrapper>
				<AgentProjectPage />
			</Wrapper>,
		);

		await screen.findByRole("heading", { name: MockChatProject.name });
		await waitFor(() => expect(authChecks).toBeDefined());
		expect(Object.values(authChecks ?? {})).toEqual(expectedPermissionChecks);
		expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
	});

	it("loads the next page of project chats", async () => {
		const user = userEvent.setup();
		const offsets: string[] = [];
		const firstPage: Chat[] = Array.from({ length: 50 }, (_, index) => ({
			...MockChat,
			id: `project-chat-${index}`,
			title: `Project chat ${index}`,
			project_id: MockChatProject.id,
		}));
		server.use(
			http.get(`/api/experimental/chats/projects/${MockChatProject.id}`, () =>
				HttpResponse.json(MockChatProject),
			),
			http.post("/api/v2/authcheck", async ({ request }) => {
				const { checks } = (await request.json()) as {
					checks: Record<string, unknown>;
				};
				return HttpResponse.json(
					Object.fromEntries(Object.keys(checks).map((key) => [key, false])),
				);
			}),
			http.get("/api/v2/chats", ({ request }) => {
				const offset = new URL(request.url).searchParams.get("offset") ?? "";
				offsets.push(offset);
				return HttpResponse.json(offset === "0" ? firstPage : []);
			}),
		);

		render(
			<Wrapper>
				<AgentProjectPage />
			</Wrapper>,
		);

		await user.click(await screen.findByRole("button", { name: "Load more" }));

		await waitFor(() => expect(offsets).toEqual(["0", "50"]));
	});

	it("shows the error when the project fails to load", async () => {
		server.use(
			http.get(`/api/experimental/chats/projects/${MockChatProject.id}`, () =>
				HttpResponse.json(
					{ message: "Project unavailable" },
					{
						status: 500,
					},
				),
			),
			http.get("/api/v2/chats", () => HttpResponse.json([])),
		);

		render(
			<Wrapper>
				<AgentProjectPage />
			</Wrapper>,
		);

		await screen.findByText("Project unavailable");
	});
});
