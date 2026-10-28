import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import { templateFoundationKey } from "#/api/queries/foundations";
import { MockTemplate, mockApiError } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import {
	MockFoundation,
	MockTemplateResourceRun,
} from "#/testHelpers/templateResources";
import { FoundationTemplateDeleteDialog } from "./FoundationTemplateDeleteDialog";

beforeEach(() => {
	vi.spyOn(API, "getTemplateFoundation").mockResolvedValue(MockFoundation);
	vi.spyOn(API, "checkAuthorization").mockResolvedValue({
		canUseFoundation: true,
	});
});
afterEach(() => vi.restoreAllMocks());

it("waits for successful destruction, then requires a separate template deletion confirmation", async () => {
	const user = userEvent.setup();
	const onConfirm = vi.fn();
	const run = vi.spyOn(API, "createFoundationRun").mockResolvedValue({
		...MockTemplateResourceRun,
		action: "destroy",
		job: { ...MockTemplateResourceRun.job, status: "pending" },
	});
	const { queryClient } = render(
		<FoundationTemplateDeleteDialog
			templateId={MockTemplate.id}
			templateName={MockTemplate.name}
			onConfirm={onConfirm}
			onCancel={vi.fn()}
		/>,
	);
	await user.click(
		await screen.findByRole("button", {
			name: "Destroy Foundation infrastructure",
		}),
	);
	await waitFor(() =>
		expect(run).toHaveBeenCalledWith(
			MockFoundation.organization_id,
			MockFoundation.id,
			{ revision_id: MockFoundation.checkpoint_revision_id, action: "destroy" },
		),
	);
	expect(onConfirm).not.toHaveBeenCalled();
	await waitFor(() => expect(queryClient.isMutating()).toBe(0));
	await act(async () => {
		queryClient.setQueryData(templateFoundationKey(MockTemplate.id), {
			...MockFoundation,
			status: "destroyed",
		});
	});
	await user.type(
		await screen.findByRole("textbox", {
			name: "Name of the template to delete",
		}),
		MockTemplate.name,
	);
	await user.click(screen.getByRole("button", { name: "Delete" }));
	expect(onConfirm).toHaveBeenCalledTimes(1);
});

it("retries cleanup of a failed initial Apply checkpoint without deleting the template", async () => {
	const user = userEvent.setup();
	const onConfirm = vi.fn();
	vi.mocked(API.getTemplateFoundation).mockResolvedValue({
		...MockFoundation,
		status: "failed",
		applied_revision_id: null,
	});
	const run = vi
		.spyOn(API, "createFoundationRun")
		.mockRejectedValueOnce(mockApiError({ message: "Cleanup unavailable." }))
		.mockResolvedValue({ ...MockTemplateResourceRun, action: "destroy" });
	const { queryClient } = render(
		<FoundationTemplateDeleteDialog
			templateId={MockTemplate.id}
			templateName={MockTemplate.name}
			onConfirm={onConfirm}
			onCancel={vi.fn()}
		/>,
	);
	await user.click(
		await screen.findByRole("button", {
			name: "Destroy Foundation infrastructure",
		}),
	);
	await waitFor(() => expect(run).toHaveBeenCalledTimes(1));
	await waitFor(() => expect(queryClient.isMutating()).toBe(0));
	expect(onConfirm).not.toHaveBeenCalled();
	await user.click(
		screen.getByRole("button", { name: "Destroy Foundation infrastructure" }),
	);
	await waitFor(() => expect(run).toHaveBeenCalledTimes(2));
	expect(run).toHaveBeenLastCalledWith(
		MockFoundation.organization_id,
		MockFoundation.id,
		{ revision_id: MockFoundation.checkpoint_revision_id, action: "destroy" },
	);
	expect(onConfirm).not.toHaveBeenCalled();
});
