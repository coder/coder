import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it } from "vitest";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import {
	MockChatProject,
	MockChatProjectMemory,
	MockChatUserMemory,
	MockDefaultOrganization,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { MemorySection } from "./MemorySection";

const Wrapper: FC<PropsWithChildren> = ({ children }) => {
	const queryClient = createTestQueryClient();
	return (
		<QueryClientProvider client={queryClient}>
			<TooltipProvider>{children}</TooltipProvider>
		</QueryClientProvider>
	);
};

afterEach(() => server.resetHandlers());

describe("MemorySection", () => {
	it("creates a project memory with a POST request", async () => {
		const user = userEvent.setup();
		let requestBody: unknown;
		server.use(
			http.get("/api/experimental/chats/projects/:projectId/memories", () =>
				HttpResponse.json([]),
			),
			http.post(
				"/api/experimental/chats/projects/:projectId/memories",
				async ({ request }) => {
					requestBody = await request.json();
					return HttpResponse.json(MockChatProjectMemory, { status: 201 });
				},
			),
		);

		render(
			<Wrapper>
				<MemorySection
					scope={{ kind: "project", projectId: MockChatProject.id }}
				/>
			</Wrapper>,
		);

		await user.click(await screen.findByRole("button", { name: "Add memory" }));
		await user.type(screen.getByLabelText("Name"), "durable-fact");
		await user.type(screen.getByLabelText("Description"), "A durable fact");
		await user.type(screen.getByLabelText("Body"), "Project memory body");
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(requestBody).toEqual({
				name: "durable-fact",
				description: "A durable fact",
				body: "Project memory body",
			});
		});
	});

	it("creates a personal memory with the selected organization", async () => {
		const user = userEvent.setup();
		let requestBody: unknown;
		server.use(
			http.get("/api/experimental/chats/memories", () => HttpResponse.json([])),
			http.post("/api/experimental/chats/memories", async ({ request }) => {
				requestBody = await request.json();
				return HttpResponse.json(MockChatUserMemory, { status: 201 });
			}),
		);

		render(
			<Wrapper>
				<MemorySection
					scope={{
						kind: "personal",
						organizationId: MockDefaultOrganization.id,
					}}
				/>
			</Wrapper>,
		);

		await user.click(await screen.findByRole("button", { name: "Add memory" }));
		await user.type(screen.getByLabelText("Name"), "durable-fact");
		await user.type(screen.getByLabelText("Description"), "A durable fact");
		await user.type(screen.getByLabelText("Body"), "Personal memory body");
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(requestBody).toEqual({
				organization_id: MockDefaultOrganization.id,
				name: "durable-fact",
				description: "A durable fact",
				body: "Personal memory body",
			});
		});
	});

	it("prefills the edit dialog with the selected project memory", async () => {
		const user = userEvent.setup();
		server.use(
			http.get("/api/experimental/chats/projects/:projectId/memories", () =>
				HttpResponse.json([MockChatProjectMemory]),
			),
		);

		render(
			<Wrapper>
				<MemorySection
					scope={{ kind: "project", projectId: MockChatProject.id }}
				/>
			</Wrapper>,
		);

		await user.click(
			await screen.findByRole("button", {
				name: new RegExp(MockChatProjectMemory.name),
				expanded: false,
			}),
		);
		await user.click(screen.getByRole("button", { name: "Edit" }));

		expect(screen.getByLabelText("Name")).toHaveValue(
			MockChatProjectMemory.name,
		);
		expect(screen.getByLabelText("Description")).toHaveValue(
			MockChatProjectMemory.description,
		);
		expect(screen.getByLabelText("Body")).toHaveValue(
			MockChatProjectMemory.body,
		);
	});

	it("clears the draft after canceling memory creation", async () => {
		const user = userEvent.setup();
		let requestBody: unknown;
		server.use(
			http.get("/api/experimental/chats/projects/:projectId/memories", () =>
				HttpResponse.json([]),
			),
			http.post(
				"/api/experimental/chats/projects/:projectId/memories",
				async ({ request }) => {
					requestBody = await request.json();
					return HttpResponse.json(MockChatProjectMemory, { status: 201 });
				},
			),
		);

		render(
			<Wrapper>
				<MemorySection
					scope={{ kind: "project", projectId: MockChatProject.id }}
				/>
			</Wrapper>,
		);

		await user.click(await screen.findByRole("button", { name: "Add memory" }));
		await user.type(screen.getByLabelText("Name"), "discarded-draft");
		await user.click(screen.getByRole("button", { name: "Cancel" }));
		await user.click(screen.getByRole("button", { name: "Add memory" }));
		await user.type(screen.getByLabelText("Name"), "durable-fact");
		await user.type(screen.getByLabelText("Description"), "A durable fact");
		await user.type(screen.getByLabelText("Body"), "Project memory body");
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(requestBody).toEqual({
				name: "durable-fact",
				description: "A durable fact",
				body: "Project memory body",
			});
		});
	});

	it("shows an error when deletion fails", async () => {
		const user = userEvent.setup();
		server.use(
			http.get("/api/experimental/chats/projects/:projectId/memories", () =>
				HttpResponse.json([MockChatProjectMemory]),
			),
			http.delete("*", () =>
				HttpResponse.json(
					{ message: "Failed to delete memory." },
					{ status: 500 },
				),
			),
		);

		render(
			<Wrapper>
				<MemorySection
					scope={{ kind: "project", projectId: MockChatProject.id }}
				/>
			</Wrapper>,
		);

		await user.click(
			await screen.findByRole("button", {
				name: new RegExp(MockChatProjectMemory.name),
				expanded: false,
			}),
		);
		await user.click(screen.getByRole("button", { name: "Delete" }));
		await user.type(
			screen.getByLabelText("Name of the memory to delete"),
			MockChatProjectMemory.name,
		);
		await user.click(screen.getByRole("button", { name: "Delete" }));

		await waitFor(() => {
			expect(screen.getByRole("alert", { hidden: true })).toHaveTextContent(
				"Failed to delete memory.",
			);
		});
	});

	it("deletes a project memory after confirmation", async () => {
		const user = userEvent.setup();
		let deletedMemoryID: string | undefined;
		let deletedMemoryURL: string | undefined;
		server.use(
			http.get("/api/experimental/chats/projects/:projectId/memories", () =>
				HttpResponse.json([MockChatProjectMemory]),
			),
			http.delete("*", ({ request }) => {
				deletedMemoryURL = request.url;
				deletedMemoryID = request.url.split("/").at(-1);
				return new HttpResponse(null, { status: 204 });
			}),
		);

		render(
			<Wrapper>
				<MemorySection
					scope={{ kind: "project", projectId: MockChatProject.id }}
				/>
			</Wrapper>,
		);

		await user.click(
			await screen.findByRole("button", {
				name: new RegExp(MockChatProjectMemory.name),
				expanded: false,
			}),
		);
		await user.click(screen.getByRole("button", { name: "Delete" }));
		await user.type(
			screen.getByLabelText("Name of the memory to delete"),
			MockChatProjectMemory.name,
		);
		await user.click(screen.getByRole("button", { name: "Delete" }));

		await waitFor(() => {
			expect(deletedMemoryURL).toContain(
				`/api/experimental/chats/projects/${MockChatProject.id}/memories/${MockChatProjectMemory.id}`,
			);
			expect(deletedMemoryID).toBe(MockChatProjectMemory.id);
		});
	});
});
