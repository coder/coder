import { Link } from "components/Link/Link";
import type { FC } from "react";

interface TermsOfServiceLinkProps {
	url?: string;
}

export const TermsOfServiceLink: FC<TermsOfServiceLinkProps> = ({ url }) => {
	return (
		<div className="pt-3 text-base">
			By continuing, you agree to the{" "}
			<Link
				className="font-medium whitespace-nowrap"
				href={url}
				target="_blank"
				rel="noreferrer"
			>
				Terms of Service
			</Link>
		</div>
	);
};
