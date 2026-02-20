import { EnterpriseBadge, PremiumBadge } from "components/Badges/Badges";
import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import { Link } from "components/Link/Link";
import { XIcon } from "lucide-react";
import type { FC } from "react";
import { docs } from "utils/docs";

type LicenseSuccessDialogProps = {
	open: boolean;
	onClose: () => void;
	licenseTier: string | null;
};

export const LicenseSuccessDialog: FC<LicenseSuccessDialogProps> = ({
	open,
	onClose,
	licenseTier,
}) => {
	const isPremium = licenseTier === "premium";
	const isEnterprise = licenseTier === "enterprise";

	if (!isPremium && !isEnterprise) {
		// This should never happen, but we'll handle it just in case.
		return null;
	}

	return (
		<Dialog open={open} onOpenChange={(o) => !o && onClose()}>
			<DialogContent>
				<DialogClose className="absolute right-4 top-4" asChild>
					<Button variant="subtle" size="icon-lg">
						<XIcon />
					</Button>
				</DialogClose>
				<DialogHeader>
					<DialogTitle className="flex justify-center items-center gap-1.5">
						Welcome to {isPremium && <PremiumBadge />}
						{isEnterprise && <EnterpriseBadge />}
					</DialogTitle>
					<DialogDescription className="text-center">
						You are now using an{" "}
						<Link
							href={docs("/admin/licensing")}
							target="_blank"
							rel="noreferrer"
						>
							{isPremium && "Premium"}
							{isEnterprise && "Enterprise"} license
						</Link>
						.
					</DialogDescription>
				</DialogHeader>
			</DialogContent>
		</Dialog>
	);
};
