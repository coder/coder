import {
	ArrowLeftIcon,
	ArrowUpRightIcon,
	BotIcon,
	KeyIcon,
	PanelLeftCloseIcon,
	ReceiptTextIcon,
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
	readonly settingsSection: string | undefined;
	readonly showApiKeysItem: boolean;
	readonly isPersonalModelOverridesEnabled: boolean;
	readonly isAdmin: boolean;
	readonly location: Location;
	readonly onCollapse?: () => void;
}

export const SettingsPanel: FC<SettingsPanelProps> = ({
	isSettingsPanel,
	settingsSection,
	showApiKeysItem,
	isPersonalModelOverridesEnabled,
	isAdmin,
	location,
	onCollapse,
}) => {
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
						Settings
					</span>
					<Button
						asChild
						variant="subtle"
						size="icon"
						aria-label="Back to agents"
						className="relative z-10 size-7 min-w-0 text-content-secondary hover:text-content-primary"
					>
						<Link to={(location.state as { from?: string })?.from || "/agents"}>
							<ArrowLeftIcon />
						</Link>
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
						to="/ai/settings/coder-agents"
						trailingIcon={ArrowUpRightIcon}
					/>
				)}
			</nav>
		</div>
	);
};
