import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ChatProjectMemoryDialog } from "./ChatProjectMemoryDialog";

describe("ChatProjectMemoryDialog", () => {
	it("submits a create request", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn(async () => {});
		render(
			<ChatProjectMemoryDialog
				open
				onOpenChange={vi.fn()}
				onSubmit={onSubmit}
			/>,
		);

		await user.type(screen.getByLabelText("Name"), "durable-fact");
		await user.type(screen.getByLabelText("Description"), "A durable fact");
		await user.type(screen.getByLabelText("Body"), "Project memory body");
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(onSubmit).toHaveBeenCalledWith({
			name: "durable-fact",
			description: "A durable fact",
			body: "Project memory body",
		});
	});

	it("blocks invalid memory names", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn(async () => {});
		render(
			<ChatProjectMemoryDialog
				open
				onOpenChange={vi.fn()}
				onSubmit={onSubmit}
			/>,
		);

		await user.type(screen.getByLabelText("Name"), "invalid name");
		await user.type(screen.getByLabelText("Body"), "Project memory body");
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(onSubmit).not.toHaveBeenCalled();
	});
});
