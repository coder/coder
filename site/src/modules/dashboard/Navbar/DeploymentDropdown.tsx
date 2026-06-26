import type { FC } from "react";
import { Link } from "react-router";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { linkToAuditing } from "#/modules/navigation";

interface DeploymentDropdownProps {
	canViewDeployment: boolean;
	canViewOrganizations: boolean;
	canViewAuditLog: boolean;
	canViewConnectionLog: boolean;
	canViewAIBridge: boolean;
	canViewAISettings: boolean;
	canViewHealth: boolean;
}

export const DeploymentDropdown: FC<DeploymentDropdownProps> = ({
	canViewDeployment,
	canViewOrganizations,
	canViewAuditLog,
	canViewConnectionLog,
	canViewAIBridge,
	canViewAISettings,
	canViewHealth,
}) => {
	if (
		!canViewAuditLog &&
		!canViewConnectionLog &&
		!canViewDeployment &&
		!canViewOrganizations &&
		!canViewAIBridge &&
		!canViewAISettings &&
		!canViewHealth
	) {
		return null;
	}

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button variant="outline" size="lg">
					Admin settings
					<ChevronDownIcon className="text-content-primary" />
				</Button>
			</DropdownMenuTrigger>

			<DropdownMenuContent align="end" className="w-[180px] min-w-auto">
				<DeploymentDropdownContent
					canViewDeployment={canViewDeployment}
					canViewOrganizations={canViewOrganizations}
					canViewAuditLog={canViewAuditLog}
					canViewConnectionLog={canViewConnectionLog}
					canViewAIBridge={canViewAIBridge}
					canViewAISettings={canViewAISettings}
					canViewHealth={canViewHealth}
				/>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

const DeploymentDropdownContent: FC<DeploymentDropdownProps> = ({
	canViewDeployment,
	canViewAuditLog,
	canViewConnectionLog,
	canViewAIBridge,
	canViewAISettings,
	canViewHealth,
}) => {
	return (
		<nav>
			{canViewDeployment && (
				<DropdownMenuItem asChild>
					<Link to="/deployment">Deployment</Link>
				</DropdownMenuItem>
			)}
			<DropdownMenuItem asChild>
				<Link to="/organizations">Organizations</Link>
			</DropdownMenuItem>
			{canViewAISettings && (
				<DropdownMenuItem asChild>
					<Link to="/ai/settings">AI</Link>
				</DropdownMenuItem>
			)}
			{canViewAuditLog && (
				<DropdownMenuItem asChild>
					<Link to={linkToAuditing}>Audit Logs</Link>
				</DropdownMenuItem>
			)}
			{canViewConnectionLog && (
				<DropdownMenuItem asChild>
					<Link to="/connectionlog">Connection Logs</Link>
				</DropdownMenuItem>
			)}
			{canViewAIBridge && (
				<DropdownMenuItem asChild>
					<Link to="/ai-gateway/sessions">AI Sessions</Link>
				</DropdownMenuItem>
			)}
			{canViewHealth && (
				<DropdownMenuItem asChild>
					<Link to="/health">Healthcheck</Link>
				</DropdownMenuItem>
			)}
		</nav>
	);
};
