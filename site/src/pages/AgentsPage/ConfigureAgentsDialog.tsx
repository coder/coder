import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import type { LucideIcon } from "lucide-react";
import { BoxesIcon, KeyRoundIcon, UserIcon, XIcon } from "lucide-react";
import { type FC, type FormEvent, useEffect, useMemo, useState } from "react";
import TextareaAutosize from "react-textarea-autosize";
import { cn } from "utils/cn";
import { ChatModelAdminPanel } from "./ChatModelAdminPanel/ChatModelAdminPanel";

type ConfigureAgentsSection = "providers" | "system-prompt" | "models";

type ConfigureAgentsSectionOption = {
	id: ConfigureAgentsSection;
	label: string;
	icon: LucideIcon;
};

interface ConfigureAgentsDialogProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	canManageChatModelConfigs: boolean;
	canSetSystemPrompt: boolean;
	systemPromptDraft: string;
	onSystemPromptDraftChange: (value: string) => void;
	onSaveSystemPrompt: (event: FormEvent) => void;
	isSystemPromptDirty: boolean;
	isDisabled: boolean;
}

export const ConfigureAgentsDialog: FC<ConfigureAgentsDialogProps> = ({
	open,
	onOpenChange,
	canManageChatModelConfigs,
	canSetSystemPrompt,
	systemPromptDraft,
	onSystemPromptDraftChange,
	onSaveSystemPrompt,
	isSystemPromptDirty,
	isDisabled,
}) => {
	const configureSectionOptions = useMemo<
		readonly ConfigureAgentsSectionOption[]
	>(() => {
		const options: ConfigureAgentsSectionOption[] = [];
		if (canManageChatModelConfigs) {
			options.push({
				id: "providers",
				label: "Providers",
				icon: KeyRoundIcon,
			});
			options.push({
				id: "models",
				label: "Models",
				icon: BoxesIcon,
			});
		}
		if (canSetSystemPrompt) {
			options.push({
				id: "system-prompt",
				label: "Behavior",
				icon: UserIcon,
			});
		}
		return options;
	}, [canManageChatModelConfigs, canSetSystemPrompt]);

	const [userActiveSection, setUserActiveSection] =
		useState<ConfigureAgentsSection>("providers");

	// Derive the effective section — validated against current options
	// every render so we never show an unavailable tab.
	const activeSection = configureSectionOptions.some(
		(s) => s.id === userActiveSection,
	)
		? userActiveSection
		: (configureSectionOptions[0]?.id ?? "providers");

	// Reset to the preferred initial section each time the dialog opens.
	useEffect(() => {
		if (open) {
			setUserActiveSection("providers");
		}
	}, [open]);

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="grid h-[min(88dvh,720px)] max-w-4xl grid-cols-1 gap-0 overflow-hidden p-0 md:grid-cols-[200px_minmax(0,1fr)]">
				{/* Visually hidden for accessibility */}
				<DialogHeader className="sr-only">
					<DialogTitle>Configure Agents</DialogTitle>
					<DialogDescription>
						Manage providers, system prompt, and available models.
					</DialogDescription>
				</DialogHeader>

				{/* Sidebar */}
				<nav className="flex flex-row gap-0.5 overflow-x-auto border-b border-border p-2 md:flex-col md:overflow-x-visible md:border-b-0 md:border-r md:p-3">
					<DialogClose asChild>
						<Button
							variant="subtle"
							size="icon"
							className="mb-2 h-8 w-8 shrink-0 border-none bg-transparent shadow-none hover:bg-surface-tertiary/30"
						>
							<XIcon className="h-[18px] w-[18px] text-content-secondary" />
							<span className="sr-only">Close</span>
						</Button>
					</DialogClose>
					{configureSectionOptions.map((section) => {
						const isActive = section.id === activeSection;
						const SectionIcon = section.icon;
						return (
							<Button
								key={section.id}
								variant="subtle"
								className={cn(
									"h-auto justify-start gap-2.5 rounded-lg border-none px-3 py-2 text-left shadow-none",
									isActive
										? "bg-surface-tertiary/50 text-content-primary hover:bg-surface-tertiary/50"
										: "bg-transparent text-content-secondary hover:bg-surface-tertiary/30 hover:text-content-primary",
								)}
								onClick={() => setUserActiveSection(section.id)}
							>
								<SectionIcon className="h-[18px] w-[18px] shrink-0" />
								<span className="text-[13px] font-medium">{section.label}</span>
							</Button>
						);
					})}
				</nav>

				{/* Content */}
				<div className="flex min-h-0 flex-col pt-5">
					<h2 className="m-0 px-6 text-xl font-semibold text-content-primary">
						{configureSectionOptions.find((s) => s.id === activeSection)
							?.label ?? "Settings"}
					</h2>

					<ScrollArea className="min-h-0 flex-1" viewportClassName="px-6 pb-6">
						{activeSection === "providers" && canManageChatModelConfigs && (
							<ChatModelAdminPanel section="providers" />
						)}
						{activeSection === "system-prompt" && canSetSystemPrompt && (
							<form
								className="space-y-4"
								onSubmit={(event) => void onSaveSystemPrompt(event)}
							>
								<p className="m-0 text-[13px] leading-relaxed text-content-secondary">
									Configure how the AI agent behaves across this deployment.
								</p>
								<div className="space-y-2">
									<h3 className="m-0 text-[13px] font-semibold text-content-primary">
										System Prompt
									</h3>
									<p className="m-0 text-xs text-content-secondary">
										Admin-only instruction applied to all new chats.
									</p>
									<TextareaAutosize
										className="min-h-[220px] w-full resize-y rounded-lg border border-border bg-surface-primary px-4 py-3 font-sans text-[13px] leading-relaxed text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-link/30"
										placeholder="Optional. Set deployment-wide instructions for all new chats."
										value={systemPromptDraft}
										onChange={(event) =>
											onSystemPromptDraftChange(event.target.value)
										}
										disabled={isDisabled}
										minRows={7}
									/>
									<div className="flex justify-end gap-2">
										<Button
											size="sm"
											variant="outline"
											type="button"
											onClick={() => onSystemPromptDraftChange("")}
											disabled={isDisabled || !systemPromptDraft}
										>
											Clear
										</Button>
										<Button
											size="sm"
											type="submit"
											disabled={isDisabled || !isSystemPromptDirty}
										>
											Save
										</Button>
									</div>
								</div>
							</form>
						)}
						{activeSection === "models" && canManageChatModelConfigs && (
							<ChatModelAdminPanel section="models" />
						)}
					</ScrollArea>
				</div>
			</DialogContent>
		</Dialog>
	);
};
