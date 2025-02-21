import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import { Link } from "components/Link/Link";
import type { FC, ReactNode } from "react";

export interface RequirePermissionProps {
	children?: ReactNode;
	isFeatureVisible: boolean;
}

/**
 * Wraps routes that are available based on RBAC or licensing.
 */
export const RequirePermission: FC<RequirePermissionProps> = ({
	children,
	isFeatureVisible,
}) => {
	if (!isFeatureVisible) {
		// return <Navigate to="/workspaces" />;
		return (
			<Dialog open={true}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>
							You don't have permission to view this page
						</DialogTitle>
					</DialogHeader>
					<DialogDescription>
						If you believe this is a mistake, please contact your administrator
						or try signing in with different credentials.
					</DialogDescription>
					<DialogFooter>
						<Link href="/">Go to workspaces</Link>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		);
	}

	return <>{children}</>;
};
