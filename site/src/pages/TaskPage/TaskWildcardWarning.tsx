import { Button } from "components/Button/Button";
import { useAuthenticated } from "hooks/useAuthenticated";
import { SquareArrowOutUpRightIcon } from "lucide-react";
import { Link as RouterLink } from "react-router";
import { docs } from "utils/docs";

export const TaskWildcardWarning = () => {
	const { permissions } = useAuthenticated();

	return (
		<div className="text-center max-w-md">
			<h3 className="font-medium text-content-primary text-base mb-3">Error</h3>
			<div className="text-content-secondary text-sm flex flex-col gap-3 items-center">
				<div className="px-4">
					This application has{" "}
					<code className="py-px px-1 bg-surface-tertiary rounded-sm text-content-primary">
						subdomain = true
					</code>
					{permissions.editDeploymentConfig ? (
						<>
							, but subdomain applications are not configured. This application
							won't be accessible until you configure the{" "}
							<code className="py-px px-1 bg-surface-tertiary rounded-sm text-content-primary whitespace-nowrap">
								--wildcard-access-url
							</code>{" "}
							flag when starting the Coder server.
						</>
					) : (
						", which requires a Coder deployment with a Wildcard access URL configured. Please contact your administrator."
					)}
				</div>
				<Button size="sm" variant="outline" asChild>
					<RouterLink to={docs("/admin/networking/wildcard-access-url")}>
						<SquareArrowOutUpRightIcon />
						Learn more about wildcard access URL
					</RouterLink>
				</Button>
			</div>
		</div>
	);
};
