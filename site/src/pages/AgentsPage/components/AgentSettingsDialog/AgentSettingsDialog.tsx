import {
	ArrowUpRightIcon,
	BotIcon,
	KeyIcon,
	ReceiptTextIcon,
	Settings2Icon,
	SettingsIcon,
	ShrinkIcon,
	UserIcon,
	XIcon,
} from "lucide-react";
import { type FC, lazy, Suspense } from "react";
import { useQuery } from "react-query";
import { useLocation } from "react-router";
import { userChatProviderConfigs } from "#/api/queries/chats";
import { Button } from "#/components/Button/Button";
import { Dialog, DialogContent, DialogTitle } from "#/components/Dialog/Dialog";
import { Loader } from "#/components/Loader/Loader";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import {
	AGENT_SETTINGS_SECTIONS,
	type AgentSettingsSection,
	withAgentSettingsSection,
} from "../../utils/agentSettingsSection";
import { SettingsNavItem } from "../ChatsSidebar/settings/SettingsNavItem";

const GeneralSection = lazy(() => import("../../AgentSettingsGeneralPage"));
const UserAgentsSection = lazy(
	() => import("../../AgentSettingsUserAgentsPage"),
);
const PersonalSkillsSection = lazy(
	() => import("../../AgentSettingsPersonalSkillsPage"),
);
const CompactionSection = lazy(
	() => import("../../AgentSettingsCompactionPage"),
);
const APIKeysSection = lazy(() => import("../../AgentSettingsAPIKeysPage"));

const sectionContent: Record<AgentSettingsSection, FC> = {
	general: GeneralSection,
	"user-agents": UserAgentsSection,
	"personal-skills": PersonalSkillsSection,
	compaction: CompactionSection,
	"api-keys": APIKeysSection,
};

const sectionLabels: Record<
	AgentSettingsSection,
	{ label: string; icon: FC<{ className?: string }> }
> = {
	general: { label: "General", icon: UserIcon },
	"user-agents": { label: "Agents", icon: BotIcon },
	"personal-skills": { label: "Personal skills", icon: ReceiptTextIcon },
	compaction: { label: "Compaction", icon: ShrinkIcon },
	"api-keys": { label: "Secrets (API keys)", icon: KeyIcon },
};

// Wraps onto multiple rows on mobile, stacked column from sm up.
const navItemClassName = "w-auto shrink-0 whitespace-nowrap sm:w-full";

type AgentSettingsDialogProps = {
	/** The open section, or undefined while the dialog is closed. */
	readonly section: AgentSettingsSection | undefined;
	readonly onClose: () => void;
	readonly isAdmin: boolean;
	readonly isPersonalModelOverridesEnabled: boolean;
	/**
	 * Whether the user can reach the deployment-level Coder Agents settings.
	 * Broader than isAdmin: organization model admins qualify without
	 * deployment config access.
	 */
	readonly canManageAgentSettings: boolean;
};

export const AgentSettingsDialog: FC<AgentSettingsDialogProps> = ({
	section,
	onClose,
	isAdmin,
	isPersonalModelOverridesEnabled,
	canManageAgentSettings,
}) => {
	const location = useLocation();
	const isOpen = section !== undefined;
	const providerConfigsQuery = useQuery({
		...userChatProviderConfigs(),
		enabled: isOpen && !isAdmin,
	});
	const showApiKeysItem =
		isAdmin ||
		section === "api-keys" ||
		Boolean(providerConfigsQuery.data?.length);
	const SectionContent = section ? sectionContent[section] : undefined;
	const visibleSections = AGENT_SETTINGS_SECTIONS.filter((candidate) => {
		if (candidate === "user-agents") {
			return isPersonalModelOverridesEnabled;
		}
		if (candidate === "api-keys") {
			return showApiKeysItem;
		}
		return true;
	});

	return (
		<Dialog
			open={isOpen}
			onOpenChange={(open) => {
				if (!open) onClose();
			}}
		>
			<DialogContent className="flex h-[85dvh] max-h-none w-[calc(100%-1.5rem)] max-w-4xl flex-col gap-0 overflow-hidden p-0">
				<div className="flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border-default px-4 py-3">
					<SettingsIcon className="size-4 shrink-0 text-content-secondary" />
					<DialogTitle className="text-base">Settings</DialogTitle>
					<div className="flex-1" />
					<Button
						variant="subtle"
						size="icon"
						onClick={onClose}
						aria-label="Close settings"
						className="size-7 min-w-0 text-content-secondary hover:text-content-primary"
					>
						<XIcon />
					</Button>
				</div>
				<div className="flex min-h-0 flex-1 flex-col sm:flex-row">
					<nav
						aria-label="Settings sections"
						className="flex shrink-0 flex-row flex-wrap gap-0.5 border-0 border-b border-solid border-border-default p-2 sm:w-56 sm:flex-col sm:flex-nowrap sm:border-b-0 sm:border-r"
					>
						{visibleSections.map((candidate) => (
							<SettingsNavItem
								key={candidate}
								icon={sectionLabels[candidate].icon}
								label={sectionLabels[candidate].label}
								active={section === candidate}
								to={{
									pathname: location.pathname,
									search: withAgentSettingsSection(location.search, candidate),
								}}
								replace
								className={navItemClassName}
							/>
						))}
						{canManageAgentSettings && (
							<SettingsNavItem
								icon={Settings2Icon}
								label="Manage agents"
								active={false}
								to="/ai/settings/coder-agents"
								trailingIcon={ArrowUpRightIcon}
								className={navItemClassName}
							/>
						)}
					</nav>
					{/* Remounting per section starts each one at the top. */}
					<ScrollArea
						key={section}
						className="min-h-0 flex-1"
						viewportClassName="[&>div]:block!"
					>
						<div className="p-6 pb-10">
							<Suspense fallback={<Loader />}>
								{SectionContent && <SectionContent />}
							</Suspense>
						</div>
					</ScrollArea>
				</div>
			</DialogContent>
		</Dialog>
	);
};
