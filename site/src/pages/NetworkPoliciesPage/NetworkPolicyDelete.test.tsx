import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NetworkPolicyDelete } from "./NetworkPolicyDelete";

it("preserves focus when queued cleanup later completes", async () => {
	const onDelete =
		vi.fn<React.ComponentProps<typeof NetworkPolicyDelete>["onDelete"]>();
	const props = {
		name: "Build egress",
		generation: 3,
		isPending: false,
		deleted: false,
		blockedReason: undefined,
		error: undefined,
		onDelete,
	};
	const { rerender } = render(<NetworkPolicyDelete {...props} />);
	const user = userEvent.setup();
	await user.click(screen.getByRole("button", { name: "Delete policy" }));
	await user.click(screen.getByRole("button", { name: "Delete" }));
	rerender(
		<NetworkPolicyDelete {...props} blockedReason="Cleanup is running." />,
	);
	const accepted = onDelete.mock.calls[0];
	if (!accepted) throw new Error("Deletion was not requested");
	await act(async () => accepted[1]());
	const section = screen.getByRole("region", { name: "Policy deletion" });
	await waitFor(() => expect(section).toHaveFocus());
	rerender(<NetworkPolicyDelete {...props} deleted />);
	expect(section).toHaveFocus();
});

it("requires confirmation and allows cancellation without deleting", async () => {
	const onDelete = vi.fn();
	render(
		<NetworkPolicyDelete
			name="Build egress"
			generation={3}
			isPending={false}
			deleted={false}
			blockedReason={undefined}
			error={undefined}
			onDelete={onDelete}
		/>,
	);
	const user = userEvent.setup();
	const trigger = screen.getByRole("button", { name: "Delete policy" });
	await user.click(trigger);
	expect(onDelete).not.toHaveBeenCalled();
	await user.click(screen.getByRole("button", { name: "Cancel" }));
	expect(onDelete).not.toHaveBeenCalled();
	await waitFor(() => expect(trigger).toHaveFocus());
	await user.click(trigger);
	await user.click(screen.getByRole("button", { name: "Delete" }));
	expect(onDelete).toHaveBeenCalledExactlyOnceWith(
		{ expected_generation: 3 },
		expect.any(Function),
	);
});

it.each(["quarantined", "changed"])(
	"blocks confirmation when the policy becomes %s while the dialog is open",
	async (reason) => {
		const onDelete = vi.fn();
		const props = {
			name: "Build egress",
			generation: 3,
			isPending: false,
			deleted: false,
			blockedReason: undefined,
			error: undefined,
			onDelete,
		};
		const { rerender } = render(<NetworkPolicyDelete {...props} />);
		const user = userEvent.setup();
		await user.click(screen.getByRole("button", { name: "Delete policy" }));
		rerender(
			<NetworkPolicyDelete
				{...props}
				generation={reason === "changed" ? 4 : 3}
				blockedReason={
					reason === "quarantined"
						? "An unknown target must be recovered before deletion."
						: undefined
				}
			/>,
		);
		await user.click(screen.getByRole("button", { name: "Delete" }));
		expect(onDelete).not.toHaveBeenCalled();
	},
);
