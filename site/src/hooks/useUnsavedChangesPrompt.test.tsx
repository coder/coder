import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { describe, expect, it } from "vitest";
import { useUnsavedChangesPrompt } from "./useUnsavedChangesPrompt";

let prompt: ReturnType<typeof useUnsavedChangesPrompt> | undefined;

const Form: React.FC = () => {
	const [reason, setReason] = useState("pending");
	prompt = useUnsavedChangesPrompt(true, reason);
	return (
		<>
			<button type="button" onClick={() => setReason("pending")}>
				Same reason
			</button>
			<button type="button" onClick={() => setReason("shown")}>
				New reason
			</button>
		</>
	);
};

describe("useUnsavedChangesPrompt", () => {
	it("closes an open prompt when the reason it opened for changes", async () => {
		const user = userEvent.setup();
		const router = createMemoryRouter([
			{ path: "/", element: <Form /> },
			{ path: "/other", element: null },
		]);
		render(<RouterProvider router={router} />);

		await act(() => router.navigate("/other"));
		expect(prompt?.isOpen).toBe(true);
		await user.click(screen.getByRole("button", { name: "Same reason" }));
		expect(prompt?.isOpen).toBe(true);
		await user.click(screen.getByRole("button", { name: "New reason" }));

		expect(prompt?.isOpen).toBe(false);
		expect(router.state.location.pathname).toBe("/");
	});
});
