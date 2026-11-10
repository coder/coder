import { cn } from "cn";
import {
	ExternalLinkIcon,
	RotateCwIcon,
	SquareArrowOutUpRightIcon,
} from "lucide-react";
import { Button } from "#/components/Button/Button";
import { NewTabLabel } from "#/components/Link/Link";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { docs } from "#/utils/docs";
import type { FrameBlock } from "./useFrameBlock";

type WorkspaceAppBlockedProps = {
	reason: { kind: "missing-wildcard" } | ({ kind: "csp" } & FrameBlock);
};

/**
 * Explains why the dashboard cannot show a workspace app in a frame, and what
 * the user or the administrator can do about it.
 */
export const WorkspaceAppBlocked: React.FC<WorkspaceAppBlockedProps> = ({
	reason,
}) => {
	const { permissions } = useAuthenticated();
	const canEditDeploymentConfig = permissions.editDeploymentConfig;

	return (
		<div className="flex size-full items-center justify-center p-4">
			<div className="text-center max-w-md">
				<h3 className="font-medium text-content-primary text-base mb-3">
					{reason.kind === "csp" ? "Your browser blocked this app" : "Error"}
				</h3>
				<div className="text-content-secondary text-sm flex flex-col gap-3 items-center">
					{reason.kind === "csp" ? (
						<CSPBlockedContent
							block={reason}
							canEditDeploymentConfig={canEditDeploymentConfig}
						/>
					) : (
						<MissingWildcardContent
							canEditDeploymentConfig={canEditDeploymentConfig}
						/>
					)}
				</div>
			</div>
		</div>
	);
};

type MissingWildcardProps = { canEditDeploymentConfig: boolean };

type CSPBlockedProps = { block: FrameBlock; canEditDeploymentConfig: boolean };

const CSPBlockedContent: React.FC<CSPBlockedProps> = ({
	block,
	canEditDeploymentConfig,
}) => {
	return (
		<>
			<div className="px-4">
				<CSPBlockedMessage
					block={block}
					canEditDeploymentConfig={canEditDeploymentConfig}
				/>
			</div>
			<div className="flex flex-wrap justify-center gap-2">
				{block.byCoderPolicy && (
					<Button size="sm" onClick={() => location.reload()}>
						<RotateCwIcon />
						Reload page
					</Button>
				)}
				<Button
					size="sm"
					variant={block.byCoderPolicy ? "outline" : "default"}
					asChild
				>
					<a href={block.src} target="_blank" rel="noreferrer">
						<ExternalLinkIcon />
						Open app in new tab
						<NewTabLabel />
					</a>
				</Button>
				{!block.byCoderPolicy && (
					<Button size="sm" variant="outline" asChild>
						<a
							href={docs(
								"/admin/networking/wildcard-access-url#apps-in-the-dashboard",
							)}
							target="_blank"
							rel="noreferrer"
						>
							<SquareArrowOutUpRightIcon />
							Learn more about blocked apps
							<NewTabLabel />
						</a>
					</Button>
				)}
			</div>
		</>
	);
};

const CSPBlockedMessage: React.FC<CSPBlockedProps> = ({
	block,
	canEditDeploymentConfig,
}) => {
	const origin = (
		<InlineCode className="wrap-anywhere">{block.origin}</InlineCode>
	);

	if (block.byCoderPolicy) {
		return (
			<>
				Coder's security policy did not allow {origin} when this page loaded.
				Reload the page to get the current policy.
			</>
		);
	}
	if (canEditDeploymentConfig) {
		return (
			<>
				A Content-Security-Policy header that Coder did not send blocks {origin}
				. A reverse proxy, load balancer or ingress in front of Coder probably
				adds it. Remove that header, or change it so that{" "}
				<InlineCode>frame-src</InlineCode> allows every host that serves
				workspace apps. The docs list these hosts.
			</>
		);
	}
	return (
		<>
			A security policy on this deployment blocks {origin}. Open the app in a
			new tab, or ask your Coder administrator to allow this address.
		</>
	);
};

const MissingWildcardContent: React.FC<MissingWildcardProps> = ({
	canEditDeploymentConfig,
}) => {
	return (
		<>
			<div className="px-4">
				This application has <InlineCode>subdomain = true</InlineCode>
				{canEditDeploymentConfig ? (
					<>
						, but subdomain applications are not configured. This application
						won't be accessible until you configure the{" "}
						<InlineCode className="whitespace-nowrap">
							--wildcard-access-url
						</InlineCode>{" "}
						flag when starting the Coder server.
					</>
				) : (
					", which requires a Coder deployment with a Wildcard Access URL configured. Please contact your administrator."
				)}
			</div>
			<Button size="sm" variant="outline" asChild>
				<a
					href={docs("/admin/networking/wildcard-access-url")}
					target="_blank"
					rel="noreferrer"
				>
					<SquareArrowOutUpRightIcon />
					Learn more about wildcard access URL
					<NewTabLabel />
				</a>
			</Button>
		</>
	);
};

const InlineCode: React.FC<React.ComponentProps<"code">> = ({
	className,
	...props
}) => (
	<code
		className={cn(
			"py-px px-1 bg-surface-tertiary rounded-sm text-content-primary",
			className,
		)}
		{...props}
	/>
);
