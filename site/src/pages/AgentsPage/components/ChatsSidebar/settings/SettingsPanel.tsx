import {
	ArrowLeftIcon,
	ArrowUpRightIcon,
	BotIcon,
	BoxesIcon,
	ChevronRightIcon,
	CoinsIcon,
	FlaskConicalIcon,
	KeyIcon,
	LayoutTemplateIcon,
	PanelLeftCloseIcon,
	PlugIcon,
	ReceiptTextIcon,
	RefreshCwIcon,
	ServerIcon,
	Settings2Icon,
	ShrinkIcon,
	UserIcon,
} from "lucide-react";
import type { FC } from "react";
import { Link, type Location } from "react-router";
import { Button } from "#/components/Button/Button";
import { cn } from "#/utils/cn";
import { SettingsNavItem } from "./SettingsNavItem";

interface SettingsPanelProps {
	readonly isSettingsPanel: boolean;
	readonly settingsPanel: "settings" | "settings-admin";
	readonly settingsSection: string | undefined;
	readonly showApiKeysItem: boolean;
	readonly isPersonalModelOverridesEnabled: boolean;
	readonly isAdmin: boolean;
	readonly location: Location;
	readonly onCollapse?: () => void;
}

export const SettingsPanel: FC<SettingsPanelProps> = ({
	isSettingsPanel,
	settingsPanel,
	settingsSection,
	showApiKeysItem,
	isPersonalModelOverridesEnabled,
	isAdmin,
	location,
	onCollapse,
}) => {
	const subNavTitle =
		settingsPanel === "settings-admin" ? "Manage agents" : "Settings";

	return (
		<div
			className={cn(
				"absolute inset-0 flex flex-col sm:transition-transform sm:duration-200 sm:ease-in-out",
				!isSettingsPanel && "translate-x-full",
			)}
			aria-hidden={!isSettingsPanel}
			inert={!isSettingsPanel ? true : undefined}
		>
			<div className="border-b border-border-default px-2 pb-2 pt-3 sm:py-2">
				<div className="relative flex items-center">
					<span className="pointer-events-none absolute inset-0 flex items-center justify-center text-sm font-medium text-content-primary">
						{subNavTitle}
					</span>
					<Button
						asChild
						variant="subtle"
						size="icon"
						aria-label={
							settingsPanel === "settings-admin"
								? "Back to settings"
								: "Back to agents"
						}
						className="relative z-10 size-7 min-w-0 text-content-secondary hover:text-content-primary"
					>
						{settingsPanel === "settings-admin" ? (
							<Link
								to="/agents/settings/general"
								state={location.state}
								aria-label="Back to settings"
							>
								<ArrowLeftIcon />
							</Link>
						) : (
							<Link
								to={(location.state as { from?: string })?.from || "/agents"}
							>
								<ArrowLeftIcon />
							</Link>
						)}
					</Button>
					<div className="flex-1" />
					{onCollapse && (
						<Button
							variant="subtle"
							size="icon"
							onClick={onCollapse}
							aria-label="Collapse sidebar"
							className="relative z-10 hidden size-7 min-w-0 text-content-secondary hover:text-content-primary sm:inline-flex"
						>
							<PanelLeftCloseIcon />
						</Button>
					)}
				</div>
			</div>
			{settingsPanel === "settings" ? (
				<nav className="flex flex-col gap-0.5 px-2 py-2">
					<SettingsNavItem
						icon={UserIcon}
						label="General"
						active={!settingsSection || settingsSection === "general"}
						to="/agents/settings/general"
						state={location.state}
					/>
					{isPersonalModelOverridesEnabled && (
						<SettingsNavItem
							icon={BotIcon}
							label="Agents"
							active={settingsSection === "user-agents"}
							to="/agents/settings/user-agents"
							state={location.state}
						/>
					)}
					<SettingsNavItem
						icon={ReceiptTextIcon}
						label="Personal skills"
						active={settingsSection === "personal-skills"}
						to="/agents/settings/personal-skills"
						state={location.state}
					/>
					<SettingsNavItem
						icon={ShrinkIcon}
						label="Compaction"
						active={settingsSection === "compaction"}
						to="/agents/settings/compaction"
						state={location.state}
					/>
					{showApiKeysItem && (
						<SettingsNavItem
							icon={KeyIcon}
							label="Secrets (API keys)"
							active={settingsSection === "api-keys"}
							to="/agents/settings/api-keys"
							state={location.state}
						/>
					)}
					{isAdmin && (
						<SettingsNavItem
							icon={Settings2Icon}
							label="Manage agents"
							active={false}
							to="/agents/settings/admin"
							state={location.state}
							trailingIcon={ChevronRightIcon}
						/>
					)}
				</nav>
			) : (
				<nav className="flex flex-col gap-0.5 px-2 py-2">
					<SettingsNavItem
						icon={BotIcon}
						label="Agents"
						active={!settingsSection || settingsSection === "agents"}
						to="/agents/settings/agents"
						state={location.state}
					/>
					<SettingsNavItem
						icon={PlugIcon}
						label="Providers"
						active={false}
						to="/ai/settings"
						trailingIcon={ArrowUpRightIcon}
					/>
					<SettingsNavItem
						icon={BoxesIcon}
						label="Models"
						active={false}
						to="/ai/settings/models"
						trailingIcon={ArrowUpRightIcon}
					/>
					<SettingsNavItem
						icon={ServerIcon}
						label="MCP servers"
						active={settingsSection === "mcp-servers"}
						to="/agents/settings/mcp-servers"
						state={location.state}
					/>
					<SettingsNavItem
						icon={LayoutTemplateIcon}
						label="Templates"
						active={settingsSection === "templates"}
						to="/agents/settings/templates"
						state={location.state}
					/>
					<SettingsNavItem
						icon={CoinsIcon}
						label="Spend"
						active={settingsSection === "spend"}
						to="/agents/settings/spend"
						state={location.state}
					/>
					<SettingsNavItem
						icon={ReceiptTextIcon}
						label="Instructions"
						active={settingsSection === "instructions"}
						to="/agents/settings/instructions"
						state={location.state}
					/>
					<SettingsNavItem
						icon={FlaskConicalIcon}
						label="Experiments"
						active={settingsSection === "experiments"}
						to="/agents/settings/experiments"
						state={location.state}
					/>
					<SettingsNavItem
						icon={RefreshCwIcon}
						label="Lifecycle"
						active={settingsSection === "lifecycle"}
						to="/agents/settings/lifecycle"
						state={location.state}
					/>
				</nav>
			)}
		</div>
	);
};
