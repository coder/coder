import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { ChatProjectTrigger } from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { MockMCPServerConfig } from "#/testHelpers/chatEntities";
import {
	MockChatProject,
	MockChatProjectACL,
	MockChatProjectExternalAuth,
	MockChatProjectGitHubMCPServerToken,
	MockChatProjectScheduleTrigger,
	MockChatProjectSlackTrigger,
	MockChatProjectTrigger,
	MockChatProjectTriggerDeliveries,
	MockChatProjectTriggerDisabled,
	MockChatSecurityAgent,
	MockGroup,
	MockRegionParameter,
	MockTemplate,
	MockUserMember,
	mockApiError,
} from "#/testHelpers/entities";
import { render, renderWithRouter } from "#/testHelpers/renderHelpers";
import { getPreferredTimezone } from "#/utils/timeZones";
import { AgentToolsSection, AgentToolsSectionView } from "./AgentToolsSection";
import { AgentTriggersSection } from "./AgentTriggersSection";
import { AgentWorkspacesSection } from "./AgentWorkspacesSection";
import { ConnectedAccountsSection } from "./ConnectedAccountsSection";
import {
	InvokableAgentsSection,
	InvokableAgentsSectionView,
} from "./InvokableAgentsSection";
import { ProjectSharingDialog } from "./ProjectSharingDialog";

const mockOtherTemplate = {
	...MockTemplate,
	id: "other-template",
	name: "other",
	display_name: "Other Template",
};

const mockTemplateRule = {
	template_id: MockTemplate.id,
	parameter_name: "region",
	allowed_values: ["us-east"],
};

const mockOtherTemplateRule = {
	...mockTemplateRule,
	template_id: mockOtherTemplate.id,
};

describe("Agent page sections", () => {
	afterEach(() => {
		vi.restoreAllMocks();
		vi.unstubAllGlobals();
	});

	it("adds a trigger from the add menu with a suggested name", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([]);
		const createTrigger = vi
			.spyOn(API.experimental, "createChatProjectTrigger")
			.mockResolvedValue({
				trigger: MockChatProjectSlackTrigger,
				webhook_url: "https://coder.example.com/hook",
			});
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", { name: "Add trigger" }),
		);
		await user.click(await screen.findByRole("option", { name: "Slack" }));
		// Typing after a click searches the Slack page.
		await user.keyboard("mention{Enter}");

		await waitFor(() =>
			expect(createTrigger).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				{
					name: "App mention",
					prompt: "",
					source: "slack",
					events: ["app_mention"],
				},
			),
		);
	});

	it("saves a chip edit at once and keeps a suggested name in step", async () => {
		const mockSuggestedNameTrigger = {
			...MockChatProjectTrigger,
			name: "Pull request opened in coder/coder",
		};
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			mockSuggestedNameTrigger,
		]);
		const updateTrigger = vi
			.spyOn(API.experimental, "updateChatProjectTrigger")
			.mockResolvedValue(mockSuggestedNameTrigger);
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", {
				name: "Events: Pull request opened",
			}),
		);
		await user.click(screen.getByRole("checkbox", { name: "Issue opened" }));

		await waitFor(() =>
			expect(updateTrigger).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				mockSuggestedNameTrigger.id,
				{
					events: ["pull_request.opened", "issues.opened"],
					name: "Pull request opened or Issue opened in coder/coder",
				},
			),
		);
	});

	it("saves a text filter and its regex switch together when the editor closes, but not on Escape", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			MockChatProjectSlackTrigger,
		]);
		const updateTrigger = vi
			.spyOn(API.experimental, "updateChatProjectTrigger")
			.mockResolvedValue(MockChatProjectSlackTrigger);
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();
		const openEditor = async () =>
			user.click(await screen.findByRole("button", { name: /^Messages: / }));

		await openEditor();
		await user.click(screen.getByRole("switch", { name: "Use regex" }));
		await user.clear(screen.getByLabelText("Text to match"));
		await user.type(screen.getByLabelText("Text to match"), "outage");
		await user.keyboard("{Escape}");
		await openEditor();
		await user.click(screen.getByRole("switch", { name: "Use regex" }));
		await user.clear(screen.getByLabelText("Text to match"));
		await user.type(screen.getByLabelText("Text to match"), "incident{Enter}");

		await waitFor(() => expect(updateTrigger).toHaveBeenCalledTimes(1));
		expect(updateTrigger).toHaveBeenCalledWith(
			MockChatProject.organization_id,
			MockChatProject.id,
			MockChatProjectSlackTrigger.id,
			{ text_pattern: "incident", text_pattern_is_regex: true },
		);
	});

	it("checks a regex before saving it", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			MockChatProjectTrigger,
		]);
		const updateTrigger = vi.spyOn(
			API.experimental,
			"updateChatProjectTrigger",
		);
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", { name: "Text filter: Any text" }),
		);
		await user.click(screen.getByRole("switch", { name: "Use regex" }));
		// RE2 flag groups are fine; the unclosed group is not.
		await user.type(
			screen.getByLabelText("Text to match"),
			"(?i)(deploy{Enter}",
		);
		await user.click(document.body);

		expect(updateTrigger).not.toHaveBeenCalled();
	});

	it("keeps a text filter the server rejects for the next try", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			MockChatProjectTrigger,
		]);
		const updateTrigger = vi
			.spyOn(API.experimental, "updateChatProjectTrigger")
			.mockRejectedValue(
				mockApiError({ message: "Lookahead is not supported in RE2." }),
			);
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();
		const openEditor = async () =>
			user.click(await screen.findByRole("button", { name: /^Text filter: / }));

		await openEditor();
		await user.click(screen.getByRole("switch", { name: "Use regex" }));
		await user.type(
			screen.getByLabelText("Text to match"),
			"(?=deploy){Enter}",
		);
		await waitFor(() => expect(updateTrigger).toHaveBeenCalledTimes(1));
		await openEditor();

		// The editor and the section both show the server's message.
		const alerts = await screen.findAllByRole("alert");
		expect(
			alerts.some((alert) =>
				alert.textContent?.includes("Lookahead is not supported in RE2."),
			),
		).toBe(true);
		expect(screen.getByLabelText("Text to match")).toHaveValue("(?=deploy)");
		expect(screen.getByRole("switch", { name: "Use regex" })).toBeChecked();
	});

	it("keeps a prompt the server rejects for the next try", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			MockChatProjectTrigger,
		]);
		const updateTrigger = vi
			.spyOn(API.experimental, "updateChatProjectTrigger")
			.mockRejectedValue(new Error("Server unavailable."));
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();
		const openEditor = async () =>
			user.click(
				await screen.findByRole("button", {
					name: new RegExp(`^Ask ${MockChatProject.name} to: `),
				}),
			);

		await openEditor();
		await user.clear(screen.getByLabelText("Prompt"));
		await user.type(screen.getByLabelText("Prompt"), "Triage the issue.");
		await user.click(screen.getByRole("button", { name: "Done" }));
		await waitFor(() => expect(updateTrigger).toHaveBeenCalledTimes(1));
		await openEditor();

		expect(await screen.findByLabelText("Prompt")).toHaveValue(
			"Triage the issue.",
		);
	});

	it("returns focus to the trigger's menu button when its delete is canceled", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			MockChatProjectTrigger,
		]);
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();
		const moreActions = await screen.findByRole("button", {
			name: `More actions for ${MockChatProjectTrigger.name}`,
		});

		await user.click(moreActions);
		await user.click(await screen.findByRole("menuitem", { name: "Delete" }));
		await user.click(await screen.findByRole("button", { name: "Cancel" }));

		await waitFor(() => expect(moreActions).toHaveFocus());
	});

	it("keeps edits queued behind a slow save", async () => {
		const mockCustomNameTrigger = {
			...MockChatProjectTrigger,
			name: "Review bot",
		};
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			mockCustomNameTrigger,
		]);
		let finishFirstSave: (value: ChatProjectTrigger) => void = () => {};
		const updateTrigger = vi
			.spyOn(API.experimental, "updateChatProjectTrigger")
			.mockImplementationOnce(
				() =>
					new Promise((resolve) => {
						finishFirstSave = resolve;
					}),
			)
			.mockImplementation(() => new Promise(() => {}));
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", {
				name: "Events: Pull request opened",
			}),
		);
		await user.click(screen.getByRole("checkbox", { name: "Issue opened" }));
		await user.click(screen.getByRole("checkbox", { name: "Push" }));
		finishFirstSave(mockCustomNameTrigger);

		// The second edit starts once the first one finishes; refetching the
		// server state in between would drop it from the list.
		await waitFor(() => expect(updateTrigger).toHaveBeenCalledTimes(2));
		expect(updateTrigger).toHaveBeenLastCalledWith(
			MockChatProject.organization_id,
			MockChatProject.id,
			mockCustomNameTrigger.id,
			{ events: ["pull_request.opened", "issues.opened", "push"] },
		);
		expect(screen.getByRole("checkbox", { name: "Push" })).toBeChecked();
		expect(
			screen.getByRole("checkbox", { name: "Issue opened" }),
		).toBeChecked();
	});

	it("puts a failed edit back and reports the error", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			MockChatProjectTrigger,
		]);
		vi.spyOn(API.experimental, "updateChatProjectTrigger").mockRejectedValue(
			new Error("Server unavailable."),
		);
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();
		const toggle = await screen.findByRole("switch", {
			name: `Enable ${MockChatProjectTrigger.name}`,
		});

		await user.click(toggle);

		expect(await screen.findByRole("alert")).toHaveTextContent(
			"Server unavailable.",
		);
		expect(toggle).toBeChecked();
	});

	it("gets a new webhook URL after confirmation", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			MockChatProjectTrigger,
		]);
		const regenerate = vi
			.spyOn(API.experimental, "regenerateChatProjectTriggerWebhookURL")
			.mockResolvedValue({
				trigger: MockChatProjectTrigger,
				webhook_url: "https://coder.example.com/new-hook",
			});
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", {
				name: `More actions for ${MockChatProjectTrigger.name}`,
			}),
		);
		await user.click(
			await screen.findByRole("menuitem", { name: "Get new webhook URL" }),
		);
		expect(regenerate).not.toHaveBeenCalled();
		await user.click(
			await screen.findByRole("button", { name: "Get new webhook URL" }),
		);

		await waitFor(() =>
			expect(regenerate).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				MockChatProjectTrigger.id,
			),
		);
	});

	it("loads a trigger's recent deliveries only when the list opens", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			MockChatProjectTrigger,
		]);
		const getDeliveries = vi
			.spyOn(API.experimental, "getChatProjectTriggerDeliveries")
			.mockResolvedValue(MockChatProjectTriggerDeliveries);
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();
		const openList = await screen.findByRole("button", {
			name: `Recent deliveries for ${MockChatProjectTrigger.name}`,
		});
		expect(getDeliveries).not.toHaveBeenCalled();

		await user.click(openList);

		await waitFor(() =>
			expect(getDeliveries).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				MockChatProjectTrigger.id,
			),
		);
	});

	it("disables a trigger from its switch", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			MockChatProjectTrigger,
		]);
		const updateTrigger = vi
			.spyOn(API.experimental, "updateChatProjectTrigger")
			.mockResolvedValue({ ...MockChatProjectTrigger, enabled: false });
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("switch", {
				name: `Enable ${MockChatProjectTrigger.name}`,
			}),
		);

		await waitFor(() =>
			expect(updateTrigger).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				MockChatProjectTrigger.id,
				{ enabled: false },
			),
		);
	});

	it("turns on continuing the same chat for a conversation", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			MockChatProjectTrigger,
		]);
		const updateTrigger = vi
			.spyOn(API.experimental, "updateChatProjectTrigger")
			.mockResolvedValue({
				...MockChatProjectTrigger,
				continue_conversations: true,
			});
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", {
				name: /: don't continue a chat$/,
			}),
		);
		await user.click(
			screen.getByRole("switch", { name: "Continue the same chat" }),
		);

		await waitFor(() =>
			expect(updateTrigger).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				MockChatProjectTrigger.id,
				{ continue_conversations: true },
			),
		);
	});

	it("lets a continued chat interrupt a busy agent", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			MockChatProjectSlackTrigger,
		]);
		const updateTrigger = vi
			.spyOn(API.experimental, "updateChatProjectTrigger")
			.mockResolvedValue({
				...MockChatProjectSlackTrigger,
				busy_behavior: "interrupt",
			});
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", {
				name: `If ${MockChatProject.name} is busy: wait for it to finish`,
			}),
		);
		await user.click(screen.getByRole("radio", { name: "Interrupt it" }));

		await waitFor(() =>
			expect(updateTrigger).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				MockChatProjectSlackTrigger.id,
				{ busy_behavior: "interrupt" },
			),
		);
	});

	it("saves a Slack signing secret", async () => {
		const mockUnverifiedSlackTrigger = {
			...MockChatProjectSlackTrigger,
			has_slack_signing_secret: false,
		};
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			mockUnverifiedSlackTrigger,
		]);
		const updateTrigger = vi
			.spyOn(API.experimental, "updateChatProjectTrigger")
			.mockResolvedValue(MockChatProjectSlackTrigger);
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", {
				name: "Slack signing secret: Not set",
			}),
		);
		await user.type(screen.getByLabelText("Signing secret"), " abc123 ");
		await user.click(screen.getByRole("button", { name: "Save secret" }));

		await waitFor(() =>
			expect(updateTrigger).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				MockChatProjectSlackTrigger.id,
				{ slack_signing_secret: "abc123" },
			),
		);
	});

	it("removes a Slack signing secret", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			MockChatProjectSlackTrigger,
		]);
		const updateTrigger = vi
			.spyOn(API.experimental, "updateChatProjectTrigger")
			.mockResolvedValue({
				...MockChatProjectSlackTrigger,
				has_slack_signing_secret: false,
			});
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", { name: "Slack signing secret: Set" }),
		);
		await user.click(screen.getByRole("button", { name: "Remove" }));

		await waitFor(() =>
			expect(updateTrigger).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				MockChatProjectSlackTrigger.id,
				{ slack_signing_secret: "" },
			),
		);
	});

	it("saves a GitHub webhook secret", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			MockChatProjectTrigger,
		]);
		const updateTrigger = vi
			.spyOn(API.experimental, "updateChatProjectTrigger")
			.mockResolvedValue({
				...MockChatProjectTrigger,
				has_github_webhook_secret: true,
			});
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", {
				name: "GitHub webhook secret: Not set",
			}),
		);
		await user.type(screen.getByLabelText("Webhook secret"), "hunter2{Enter}");

		await waitFor(() =>
			expect(updateTrigger).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				MockChatProjectTrigger.id,
				{ github_webhook_secret: "hunter2" },
			),
		);
	});

	it("adds a schedule trigger with its schedule and prompt", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([]);
		const createTrigger = vi
			.spyOn(API.experimental, "createChatProjectTrigger")
			.mockResolvedValue({
				trigger: MockChatProjectScheduleTrigger,
				webhook_url: "",
			});
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", { name: "Add trigger" }),
		);
		await user.click(await screen.findByRole("option", { name: "Schedule" }));
		const dialog = await screen.findByRole("dialog");
		await user.click(within(dialog).getByLabelText("Prompt"));
		await user.paste("Summarize merged pull requests.");
		await user.click(
			within(dialog).getByRole("button", { name: "Add trigger" }),
		);

		await waitFor(() =>
			expect(createTrigger).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				{
					name: "Every weekday at 9:00 AM",
					prompt: "Summarize merged pull requests.",
					source: "schedule",
					schedule: `CRON_TZ=${getPreferredTimezone()} 0 9 * * 1-5`,
				},
			),
		);
	});

	it("saves a new time for a schedule trigger and keeps its suggested name in step", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			MockChatProjectScheduleTrigger,
		]);
		const updateTrigger = vi
			.spyOn(API.experimental, "updateChatProjectTrigger")
			.mockResolvedValue(MockChatProjectScheduleTrigger);
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", { name: /^Schedule: / }),
		);
		const time = screen.getByLabelText("Time");
		await user.clear(time);
		await user.type(time, "10:30{Enter}");

		await waitFor(() =>
			expect(updateTrigger).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				MockChatProjectScheduleTrigger.id,
				{
					schedule: "CRON_TZ=UTC 30 10 * * 1-5",
					name: "Every weekday at 10:30 AM",
				},
			),
		);
	});

	it("runs a schedule trigger now", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			MockChatProjectScheduleTrigger,
		]);
		const runTrigger = vi
			.spyOn(API.experimental, "runChatProjectTrigger")
			.mockResolvedValue({ chat_id: "run-1", continued: false });
		const toastSuccess = vi.spyOn(toast, "success");
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", {
				name: `More actions for ${MockChatProjectScheduleTrigger.name}`,
			}),
		);
		await user.click(await screen.findByRole("menuitem", { name: "Run now" }));

		await waitFor(() =>
			expect(runTrigger).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				MockChatProjectScheduleTrigger.id,
			),
		);
		await waitFor(() =>
			expect(toastSuccess).toHaveBeenCalledWith(
				`Started a chat from "${MockChatProjectScheduleTrigger.name}".`,
				expect.anything(),
			),
		);
	});

	it("starts one run when Run now is chosen again while a run is starting", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([
			MockChatProjectScheduleTrigger,
		]);
		const runTrigger = vi
			.spyOn(API.experimental, "runChatProjectTrigger")
			.mockReturnValue(new Promise(() => undefined));
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();
		const runNow = async () => {
			await user.click(
				await screen.findByRole("button", {
					name: `More actions for ${MockChatProjectScheduleTrigger.name}`,
				}),
			);
			await user.click(
				await screen.findByRole("menuitem", { name: /Run now/ }),
			);
		};

		await runNow();
		await waitFor(() => expect(runTrigger).toHaveBeenCalledTimes(1));
		await runNow();

		expect(runTrigger).toHaveBeenCalledTimes(1);
	});

	it("keeps a new webhook URL when a schedule trigger is added after it", async () => {
		vi.spyOn(API.experimental, "getChatProjectTriggers").mockResolvedValue([]);
		const webhookURL = "https://coder.example.com/hook-shown-once";
		vi.spyOn(API.experimental, "createChatProjectTrigger")
			.mockResolvedValueOnce({
				trigger: MockChatProjectTriggerDisabled,
				webhook_url: webhookURL,
			})
			.mockResolvedValueOnce({
				trigger: MockChatProjectScheduleTrigger,
				webhook_url: "",
			});
		render(<AgentTriggersSection project={MockChatProject} />);
		const user = userEvent.setup();
		const addTrigger = async (source: string) => {
			await user.click(
				await screen.findByRole("button", { name: "Add trigger" }),
			);
			await user.click(await screen.findByRole("option", { name: source }));
		};

		await addTrigger("Webhook");
		await screen.findByDisplayValue(webhookURL);
		await addTrigger("Schedule");
		const dialog = await screen.findByRole("dialog");
		await user.click(within(dialog).getByLabelText("Prompt"));
		await user.paste("Summarize.");
		await user.click(
			within(dialog).getByRole("button", { name: "Add trigger" }),
		);
		await waitFor(() =>
			expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
		);

		expect(screen.getByDisplayValue(webhookURL)).toBeInTheDocument();
	});

	it("adds another agent to the invokable list, leaving out itself, added agents, and agents the user cannot edit", async () => {
		const mockAddedAgent = {
			...MockChatProject,
			id: "added",
			name: "Docs writer",
		};
		const mockReadOnlyAgent = {
			...MockChatProject,
			id: "read-only",
			name: "Read-only agent",
			permissions: { ...MockChatProject.permissions, edit: false },
		};
		const mockAgentInvokingDocs = {
			...MockChatProject,
			invokable_agent_ids: [mockAddedAgent.id],
		};
		vi.spyOn(API.experimental, "getChatProjects").mockResolvedValue([
			MockChatProject,
			MockChatSecurityAgent,
			mockAddedAgent,
			mockReadOnlyAgent,
		]);
		const updateProject = vi
			.spyOn(API.experimental, "updateChatProject")
			.mockResolvedValue(MockChatProject);
		render(<InvokableAgentsSection project={mockAgentInvokingDocs} />);
		const user = userEvent.setup();

		await user.click(await screen.findByRole("button", { name: "Add agent" }));
		const options = await screen.findAllByRole("option");
		expect(options.map((option) => option.textContent)).toEqual([
			MockChatSecurityAgent.name,
		]);
		await user.click(options[0]);

		await waitFor(() =>
			expect(updateProject).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				{ invokable_agent_ids: [mockAddedAgent.id, MockChatSecurityAgent.id] },
			),
		);
	});

	it("removes an agent from the invokable list", async () => {
		const mockAgentInvokingSecurity = {
			...MockChatProject,
			invokable_agent_ids: [MockChatSecurityAgent.id],
		};
		vi.spyOn(API.experimental, "getChatProjects").mockResolvedValue([
			MockChatProject,
			MockChatSecurityAgent,
		]);
		const updateProject = vi
			.spyOn(API.experimental, "updateChatProject")
			.mockResolvedValue(MockChatProject);
		render(<InvokableAgentsSection project={mockAgentInvokingSecurity} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", {
				name: `Remove ${MockChatSecurityAgent.name}`,
			}),
		);

		await waitFor(() =>
			expect(updateProject).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				{ invokable_agent_ids: [] },
			),
		);
	});

	it("ignores Remove and Add agent while a change saves", async () => {
		const onAddAgent = vi.fn();
		const onRemoveAgent = vi.fn();
		render(
			<InvokableAgentsSectionView
				invokable={[MockChatSecurityAgent]}
				available={[MockChatProject]}
				agentsError={undefined}
				onRetryAgents={vi.fn()}
				canEdit
				isSaving
				saveError={undefined}
				onAddAgent={onAddAgent}
				onRemoveAgent={onRemoveAgent}
			/>,
		);
		const user = userEvent.setup();

		await user.click(
			screen.getByRole("button", {
				name: `Remove ${MockChatSecurityAgent.name}`,
			}),
		);
		await user.click(screen.getByRole("button", { name: "Add agent" }));

		expect(onRemoveAgent).not.toHaveBeenCalled();
		expect(onAddAgent).not.toHaveBeenCalled();
		expect(screen.queryByPlaceholderText("Search agents")).toBeNull();
	});

	it("ignores Remove and Add MCP server while a change saves", async () => {
		const onAdd = vi.fn();
		const onRemove = vi.fn();
		render(
			<AgentToolsSectionView
				attached={[{ id: "attached", label: "GitHub", iconURL: "" }]}
				available={[{ id: "available", label: "Linear", iconURL: "" }]}
				isReady
				error={undefined}
				onRetry={vi.fn()}
				canEdit
				isSaving
				saveError={undefined}
				onAdd={onAdd}
				onRemove={onRemove}
			/>,
		);
		const user = userEvent.setup();

		await user.click(screen.getByRole("button", { name: "Remove GitHub" }));
		await user.click(screen.getByRole("button", { name: "Add MCP server" }));

		expect(onRemove).not.toHaveBeenCalled();
		expect(onAdd).not.toHaveBeenCalled();
		expect(screen.queryByPlaceholderText("Search MCP servers")).toBeNull();
	});

	it("changes and removes a sharing role", async () => {
		vi.spyOn(API.experimental, "getChatProjectACL").mockResolvedValue(
			MockChatProjectACL,
		);
		const updateACL = vi
			.spyOn(API.experimental, "updateChatProjectACL")
			.mockResolvedValue();
		render(
			<ProjectSharingDialog project={MockChatProject} onClose={vi.fn()} />,
		);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("combobox", {
				name: `Role for ${MockUserMember.name || MockUserMember.username}`,
			}),
		);
		await user.click(await screen.findByRole("option", { name: /Can use/ }));
		await waitFor(() =>
			expect(updateACL).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				{ user_roles: { [MockUserMember.id]: "use" } },
			),
		);

		await user.click(
			screen.getByRole("button", { name: `Remove ${MockGroup.display_name}` }),
		);
		await waitFor(() =>
			expect(updateACL).toHaveBeenLastCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				{ group_roles: { [MockGroup.id]: "" } },
			),
		);
	});

	it("saves a GitHub App credential for the agent's GitHub server", async () => {
		vi.spyOn(API.experimental, "getChatProjectExternalAuth").mockResolvedValue({
			providers: [],
			links: [],
		});
		vi.spyOn(
			API.experimental,
			"getChatProjectMCPServerTokens",
		).mockResolvedValue([MockChatProjectGitHubMCPServerToken]);
		vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
		const saveToken = vi
			.spyOn(API.experimental, "updateChatProjectMCPServerToken")
			.mockResolvedValue({ has_token: true });
		render(<ConnectedAccountsSection project={MockChatProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", {
				name: "Add credential for GitHub",
			}),
		);
		await user.click(screen.getByRole("radio", { name: "Use a GitHub App" }));
		await user.type(screen.getByLabelText("App ID"), "123");
		await user.type(screen.getByLabelText("Installation ID"), "456");
		await user.type(screen.getByLabelText("Private key"), "pem");
		await user.click(screen.getByRole("button", { name: "Save credential" }));

		await waitFor(() =>
			expect(saveToken).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				"mcp-github",
				{
					credential_kind: "github_app",
					github_app: {
						app_id: 123,
						installation_id: 456,
						private_key: "pem",
						repositories: [],
						use_for_git: false,
					},
				},
			),
		);
	});

	it("connects an account and continues to the provider", async () => {
		vi.spyOn(API.experimental, "getChatProjectExternalAuth").mockResolvedValue({
			...MockChatProjectExternalAuth,
			links: [],
		});
		const connect = vi
			.spyOn(API.experimental, "connectChatProjectExternalAuth")
			.mockResolvedValue({ url: "/external-auth/github/callback" });
		const location = { href: "http://localhost/agents" };
		vi.stubGlobal("location", location);
		render(<ConnectedAccountsSection project={MockChatProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", { name: "Connect GitHub" }),
		);

		await waitFor(() =>
			expect(connect).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				"github",
				{
					redirect: `/agents/projects/${MockChatProject.id}/settings?connected=github`,
				},
			),
		);
		await waitFor(() =>
			expect(location.href).toBe("/external-auth/github/callback"),
		);
	});

	it("confirms a connected account when the provider flow returns", async () => {
		vi.spyOn(API.experimental, "getChatProjectExternalAuth").mockResolvedValue(
			MockChatProjectExternalAuth,
		);
		const toastSuccess = vi.spyOn(toast, "success");
		renderWithRouter(
			createMemoryRouter(
				[
					{
						path: "*",
						element: <ConnectedAccountsSection project={MockChatProject} />,
					},
				],
				{
					initialEntries: ["/agents/projects/p/settings?connected=github"],
				},
			),
		);

		await waitFor(() =>
			expect(toastSuccess).toHaveBeenCalledWith("Account connected"),
		);
		expect(toastSuccess).toHaveBeenCalledTimes(1);
	});

	it("disconnects an account after confirmation", async () => {
		vi.spyOn(API.experimental, "getChatProjectExternalAuth").mockResolvedValue(
			MockChatProjectExternalAuth,
		);
		const disconnect = vi
			.spyOn(API.experimental, "deleteChatProjectExternalAuth")
			.mockResolvedValue({ token_revoked: false });
		render(<ConnectedAccountsSection project={MockChatProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", { name: "Disconnect GitHub" }),
		);
		await user.click(
			within(await screen.findByRole("dialog")).getByRole("button", {
				name: "Disconnect",
			}),
		);

		await waitFor(() =>
			expect(disconnect).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				"github",
			),
		);
	});

	const mockAccessOptions = () => {
		vi.spyOn(API, "getTemplates").mockResolvedValue([
			MockTemplate,
			mockOtherTemplate,
		]);
		vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
		vi.spyOn(API, "getUserExternalAuthProviders").mockResolvedValue({
			providers: [],
			links: [],
		});
		return vi
			.spyOn(API.experimental, "updateChatProject")
			.mockResolvedValue(MockChatProject);
	};
	it("turns workspace tools off from the Workspaces switch", async () => {
		const updateProject = mockAccessOptions();
		const mockProject = {
			...MockChatProject,
			organization_id: MockTemplate.organization_id,
		};
		render(<AgentWorkspacesSection project={mockProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("switch", { name: "Workspace tools" }),
		);

		await waitFor(() =>
			expect(updateProject).toHaveBeenCalledWith(
				mockProject.organization_id,
				mockProject.id,
				{ workspace_tools_enabled: false },
			),
		);
	});

	it("drops a template's parameter rules when it is no longer allowed", async () => {
		const updateProject = mockAccessOptions();
		const mockProject = {
			...MockChatProject,
			organization_id: MockTemplate.organization_id,
			allowed_template_ids: [MockTemplate.id, mockOtherTemplate.id],
			template_parameter_rules: [mockTemplateRule, mockOtherTemplateRule],
		};
		render(<AgentWorkspacesSection project={mockProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("checkbox", {
				name: MockTemplate.display_name || MockTemplate.name,
			}),
		);

		await waitFor(() =>
			expect(updateProject).toHaveBeenCalledWith(
				mockProject.organization_id,
				mockProject.id,
				{
					allowed_template_ids: [mockOtherTemplate.id],
					template_parameter_rules: [mockOtherTemplateRule],
				},
			),
		);
	});

	it("attaches an MCP server as soon as it is picked", async () => {
		const updateProject = mockAccessOptions();
		vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([
			MockMCPServerConfig,
		]);
		const mockProject = {
			...MockChatProject,
			organization_id: MockTemplate.organization_id,
		};
		render(<AgentToolsSection project={mockProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", { name: "Add MCP server" }),
		);
		await user.click(
			await screen.findByRole("option", {
				name: MockMCPServerConfig.display_name || MockMCPServerConfig.slug,
			}),
		);

		await waitFor(() =>
			expect(updateProject).toHaveBeenCalledWith(
				mockProject.organization_id,
				mockProject.id,
				{ mcp_server_ids: [MockMCPServerConfig.id] },
			),
		);
	});

	it("limits a template parameter to the chosen values", async () => {
		const updateProject = mockAccessOptions();
		vi.spyOn(API, "getTemplateVersionRichParameters").mockResolvedValue([
			MockRegionParameter,
		]);
		const mockProject = {
			...MockChatProject,
			organization_id: MockTemplate.organization_id,
			template_parameter_rules: [mockOtherTemplateRule],
		};
		render(<AgentWorkspacesSection project={mockProject} />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", {
				name: `Configure parameters for ${MockTemplate.display_name || MockTemplate.name}`,
			}),
		);
		await user.click(
			await screen.findByRole("checkbox", { name: "Restrict Region" }),
		);
		// The template default is preselected; allow EU too and make it the
		// default.
		await user.click(screen.getByRole("checkbox", { name: "EU West" }));
		await user.click(
			screen.getByRole("combobox", { name: "Default value for Region" }),
		);
		await user.click(await screen.findByRole("option", { name: "EU West" }));
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() =>
			expect(updateProject).toHaveBeenCalledWith(
				mockProject.organization_id,
				mockProject.id,
				{
					template_parameter_rules: [
						mockOtherTemplateRule,
						{
							template_id: MockTemplate.id,
							parameter_name: "region",
							allowed_values: ["eu-west", "us-east"],
						},
					],
				},
			),
		);
	});
});
