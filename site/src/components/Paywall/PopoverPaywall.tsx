import type { FC, ReactNode } from "react";
import { PaywallPremium } from "./PaywallPremium";

interface PopoverPaywallProps {
	message: string;
	description?: ReactNode;
	documentationLink: string;
}

export const PopoverPaywall: FC<PopoverPaywallProps> = ({
	message,
	description,
	documentationLink,
}) => {
	return (
		<PaywallPremium
			message={message}
			description={description}
			documentationLink={documentationLink}
			compact={true}
		/>
	);
};
