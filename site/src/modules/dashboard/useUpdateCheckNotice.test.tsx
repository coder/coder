import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC } from "react";
import { Toaster } from "#/components/Toaster/Toaster";
import { useUpdateCheckNotice } from "./useUpdateCheckNotice";

const version = "v0.12.9";
const releaseNotesUrl = "https://github.com/coder/coder/releases/tag/v0.12.9";

const Harness: FC<{ onDismiss: () => void; isVisible?: boolean }> = ({
	onDismiss,
	isVisible = true,
}) => {
	useUpdateCheckNotice({
		isVisible,
		version,
		releaseNotesUrl,
		onDismiss,
	});
	// The Toaster must be mounted before the notice emits so sonner delivers it.
	return <Toaster />;
};

it("shows the update toast with release and upgrade links", async () => {
	render(<Harness onDismiss={() => {}} />);

	await expect(
		screen.findByText(/Coder v0\.12\.9 is now available/),
	).resolves.toBeInTheDocument();
	expect(
		screen.getByRole("link", { name: "View release notes" }),
	).toHaveAttribute("href", releaseNotesUrl);
	expect(
		screen.getByRole("link", { name: "View upgrade instructions" }),
	).toBeInTheDocument();
});

it("persists dismissal when the close button is clicked", async () => {
	const onDismiss = vi.fn();
	render(<Harness onDismiss={onDismiss} />);

	await screen.findByText(/Coder v0\.12\.9 is now available/);
	await userEvent.click(screen.getByRole("button", { name: "Close toast" }));

	await waitFor(() => expect(onDismiss).toHaveBeenCalled());
});

it("does not show the toast when not visible", () => {
	render(<Harness onDismiss={() => {}} isVisible={false} />);

	expect(
		screen.queryByText(/Coder v0\.12\.9 is now available/),
	).not.toBeInTheDocument();
});
