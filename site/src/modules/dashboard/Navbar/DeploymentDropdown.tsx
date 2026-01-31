import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { ChevronDownIcon } from "lucide-react";
import { linkToAuditing } from "modules/navigation";
import type { FC } from "react";
import { Link } from "react-router";

interface DeploymentDropdownProps {
	canViewDeployment: boolean;
	canViewOrganizations: boolean;
	canViewAuditLog: boolean;
	canViewConnectionLog: boolean;
	canViewHealth: boolean;
	canViewAIBridge: boolean;
}

export const DeploymentDropdown: FC<DeploymentDropdownProps> = ({
	canViewDeployment,
	canViewOrganizations,
	canViewAuditLog,
	canViewConnectionLog,
	canViewHealth,
	canViewAIBridge,
}) => {
	if (
		!canViewAuditLog &&
		!canViewConnectionLog &&
		!canViewOrganizations &&
		!canViewDeployment &&
		!canViewHealth &&
		!canViewAIBridge
	) {
		return null;
	}

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button variant="outline" size="lg">
					Admin settings
					<ChevronDownIcon className="text-content-primary size-icon-sm text-white" />
				</Button>
			</DropdownMenuTrigger>

			<DropdownMenuContent align="end" className="w-[180px] min-w-auto">
				<DeploymentDropdownContent
					canViewDeployment={canViewDeployment}
					canViewOrganizations={canViewOrganizations}
					canViewAuditLog={canViewAuditLog}
					canViewConnectionLog={canViewConnectionLog}
					canViewHealth={canViewHealth}
					canViewAIBridge={canViewAIBridge}
				/>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

const DeploymentDropdownContent: FC<DeploymentDropdownProps> = ({
	canViewDeployment,
	canViewOrganizations,
	canViewAuditLog,
	canViewHealth,
	canViewConnectionLog,
	canViewAIBridge,
}) => {
	return (
		<nav>
			{canViewDeployment && (
				<DropdownMenuItem asChild>
					<Link to="/deployment">Deployment</Link>
				</DropdownMenuItem>
			)}
			{canViewOrganizations && (
				<DropdownMenuItem asChild>
					<Link to="/organizations">Organizations</Link>
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
					<Link to="/aibridge">AI Bridge Logs</Link>
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
