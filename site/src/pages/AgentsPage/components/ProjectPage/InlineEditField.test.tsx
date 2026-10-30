import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { mockApiError } from "#/testHelpers/entities";
import { InlineEditField } from "./InlineEditField";

const renderField = (
	props: Partial<React.ComponentProps<typeof InlineEditField>> = {},
) => {
	const onSave = vi.fn().mockResolvedValue(undefined);
	render(
		<InlineEditField
			label="Name"
			value="Launch"
			onSave={onSave}
			validate={(v) => (v.trim() === "" ? "Name is required." : undefined)}
			{...props}
		/>,
	);
	return { onSave, user: userEvent.setup() };
};

const startEditing = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(screen.getByRole("button", { name: "Edit Name" }));
	const input = screen.getByRole("textbox", { name: "Name" });
	await user.clear(input);
	return input;
};

describe("InlineEditField", () => {
	it("saves the draft on Enter", async () => {
		const { onSave, user } = renderField();

		await startEditing(user);
		await user.keyboard("Ship it{Enter}");

		await waitFor(() => expect(onSave).toHaveBeenCalledWith("Ship it"));
	});

	it("saves when focus leaves the field", async () => {
		const { onSave, user } = renderField();

		await startEditing(user);
		await user.keyboard("Ship it");
		await user.tab();
		await user.tab();
		await user.tab();

		await waitFor(() => expect(onSave).toHaveBeenCalledWith("Ship it"));
	});

	it("restores the saved value on Escape without saving", async () => {
		const { onSave, user } = renderField();

		await startEditing(user);
		await user.keyboard("Ship it{Escape}");

		await screen.findByRole("button", { name: "Edit Name" });
		expect(onSave).not.toHaveBeenCalled();
		await user.click(screen.getByRole("button", { name: "Edit Name" }));
		expect(screen.getByRole("textbox", { name: "Name" })).toHaveValue("Launch");
	});

	it("does not save an invalid draft", async () => {
		const { onSave, user } = renderField();

		await startEditing(user);
		await user.keyboard("{Enter}");

		expect(onSave).not.toHaveBeenCalled();
	});

	it("stays open and shows the error when saving fails", async () => {
		const onSave = vi
			.fn()
			.mockRejectedValue(mockApiError({ message: "Name is taken." }));
		const { user } = renderField({ onSave });

		await startEditing(user);
		await user.keyboard("Ship it{Enter}");

		await waitFor(() => expect(onSave).toHaveBeenCalledWith("Ship it"));
		expect(await screen.findByRole("textbox", { name: "Name" })).toHaveValue(
			"Ship it",
		);
	});

	it("clears a failed save's error after cancelling and editing again", async () => {
		const onSave = vi
			.fn()
			.mockRejectedValue(mockApiError({ message: "Name is taken." }));
		const { user } = renderField({ onSave });

		await startEditing(user);
		await user.keyboard("Ship it{Enter}");
		const input = await screen.findByRole("textbox", { name: "Name" });
		await waitFor(() => expect(input).toHaveAccessibleDescription());
		await user.keyboard("{Escape}");

		await user.click(await screen.findByRole("button", { name: "Edit Name" }));

		expect(screen.getByRole("textbox", { name: "Name" })).not.toHaveAttribute(
			"aria-invalid",
			"true",
		);
		expect(screen.queryByRole("alert")).not.toBeInTheDocument();
	});
});
