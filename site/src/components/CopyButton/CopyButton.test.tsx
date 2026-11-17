import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

	it("keeps the confirmation until the user's next key press", async () => {
		const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
		vi.useFakeTimers();
		try {
			renderComponent(
				<CopyButton text="crimson-cobra-7" label="Copy workspace name" />,
			);
			const status = screen.getByRole("status");
			await user.click(
				screen.getByRole("button", { name: "Copy workspace name" }),
			);

			act(() => vi.advanceTimersByTime(60_000));
			expect(status).toHaveTextContent("Copied");

			await user.keyboard("{Tab}");
			expect(status).toBeEmptyDOMElement();
		} finally {
			vi.useRealTimers();
		}
	});
});
