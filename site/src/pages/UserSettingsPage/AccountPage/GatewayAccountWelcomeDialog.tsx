import { SparklesIcon, XIcon } from "lucide-react";
import { type FC, useEffect, useState } from "react";
import { Link as RouterLink } from "react-router";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Link } from "#/components/Link/Link";

/**
 * dismissalStorageKey returns the localStorage key used to remember that a
 * specific user has dismissed the welcome dialog. The key is scoped to the
 * user id so the dialog reappears for each new Gateway Account on first
 * sign-in.
 */
const dismissalStorageKey = (userID: string): string =>
	`coder.gatewayAccountWelcomeDismissed.${userID}`;

interface GatewayAccountWelcomeDialogProps {
	userID: string;
}

export const GatewayAccountWelcomeDialog: FC<
	GatewayAccountWelcomeDialogProps
> = ({ userID }) => {
	const [open, setOpen] = useState(false);

	useEffect(() => {
		if (!userID) {
			return;
		}
		try {
			if (localStorage.getItem(dismissalStorageKey(userID)) === "true") {
				return;
			}
		} catch {
			// Local storage may be unavailable (private mode, quota). In that
			// case we still show the dialog; we just won't remember the
			// dismissal across sessions.
		}
		setOpen(true);
	}, [userID]);

	const handleOpenChange = (nextOpen: boolean) => {
		setOpen(nextOpen);
		if (!nextOpen && userID) {
			try {
				localStorage.setItem(dismissalStorageKey(userID), "true");
			} catch {
				// See above: ignore storage errors.
			}
		}
	};

	return (
		<Dialog open={open} onOpenChange={handleOpenChange}>
			<DialogContent className="max-w-md">
				<DialogClose
					className="absolute right-4 top-4 rounded-sm bg-transparent border-0 p-1 text-content-secondary hover:text-content-primary cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link"
					aria-label="Close"
				>
					<XIcon aria-hidden className="size-icon-sm" />
				</DialogClose>
				<DialogHeader>
					<div className="flex size-10 items-center justify-center rounded-md bg-surface-secondary">
						<SparklesIcon
							aria-hidden
							className="size-icon-sm text-content-primary"
						/>
					</div>
					<DialogTitle>You&rsquo;re signed in as a Gateway Account</DialogTitle>
					<DialogDescription className="text-content-secondary">
						Gateway Accounts are designed for AI tool access outside of Coder
						workspaces. You can generate session tokens, view your AI usage and
						spend, and configure tools like Claude Code or Cursor to route
						through AI Gateway. Workspace and template access isn&rsquo;t
						included.
					</DialogDescription>
				</DialogHeader>
				<div>
					<DialogClose asChild>
						<Link asChild showExternalIcon={false}>
							<RouterLink to="/settings/tokens/new">
								Add your first token
							</RouterLink>
						</Link>
					</DialogClose>
				</div>
			</DialogContent>
		</Dialog>
	);
};
