import { Link } from "components/Link/Link";
import type { FC } from "react";

interface TermsOfServiceLinkProps {
	className?: string;
	url?: string;
}

export const TermsOfServiceLink: FC<TermsOfServiceLinkProps> = ({
	className,
	url,
}) => {
	return (
		<div className={`pt-3 text-base ${className || ""}`}>
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
