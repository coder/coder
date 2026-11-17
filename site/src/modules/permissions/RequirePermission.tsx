import { useCallback } from "react";
import { Button } from "#/components/Button/Button";
import { NoAccessIllustration } from "#/components/EmptyState/components/NoAccessIllustration";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Link } from "#/components/Link/Link";

type RequirePermissionProps = {
	children?: React.ReactNode;
	isFeatureVisible: boolean;
};

/**
 * Wraps routes that are available based on RBAC or licensing.
 */
export const RequirePermission: React.FC<RequirePermissionProps> = ({
	children,
	isFeatureVisible,
}) => {
	const focusReturnLink = useCallback((link: HTMLAnchorElement | null) => {
		link?.focus();
	}, []);

	if (!isFeatureVisible) {
		return (
			<div className="w-full h-full flex justify-center items-center">
				<EmptyState
					image={<NoAccessIllustration className="-mb-5 h-auto max-w-full" />}
					message="You don't have permission to view this page"
					description="If you believe this is a mistake, please contact your administrator or try signing in with different credentials."
					cta={
						<Button asChild>
							<Link ref={focusReturnLink} href="/">
								Go to workspaces
							</Link>
						</Button>
					}
				/>
			</div>
		);
	}

	return children;
};
