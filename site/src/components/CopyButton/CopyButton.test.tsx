import { act, fireEvent, screen } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { onTestFinished } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { CopyButton } from "./CopyButton";

describe("CopyButton", () => {
	it("announces a successful copy", async () => {
		const user = userEvent.setup();
		renderComponent(
			<CopyButton text="crimson-cobra-7" label="Copy workspace name" />,
		);

		const status = screen.getByRole("status");
		expect(status).toBeEmptyDOMElement();
		await user.click(
			screen.getByRole("button", { name: "Copy workspace name" }),
		);

		expect(status).toHaveTextContent("Copied");
	});

	it.each([
		["key press", (user: UserEvent) => user.keyboard("{Tab}")],
		[
			"pointer press",
			(user: UserEvent) =>
				user.pointer({ keys: "[MouseLeft]", target: document.body }),
		],
		// fireEvent, because user.click sends pointerdown first.
		["click", async () => fireEvent.click(document.body)],
	])("keeps the confirmation until the user's next %s", async (_, dismiss) => {
		vi.useFakeTimers();
		onTestFinished(() => {
			vi.useRealTimers();
		});
		const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
		renderComponent(
			<CopyButton text="crimson-cobra-7" label="Copy workspace name" />,
		);
		const status = screen.getByRole("status");
		await user.click(
			screen.getByRole("button", { name: "Copy workspace name" }),
		);

		act(() => vi.advanceTimersByTime(60_000));
		await user.keyboard("{Shift}");
		expect(status).toHaveTextContent("Copied");

		await dismiss(user);
		expect(status).toBeEmptyDOMElement();

		await user.click(
			screen.getByRole("button", { name: "Copy workspace name" }),
		);
		await vi.waitFor(() => expect(status).toHaveTextContent("Copied"));
	});
});
