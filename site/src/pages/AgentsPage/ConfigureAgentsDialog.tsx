import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import type { LucideIcon } from "lucide-react";
import {
	BoxesIcon,
	KeyRoundIcon,
	MessageSquareTextIcon,
	ShieldIcon,
	UserIcon,
	XIcon,
} from "lucide-react";
import { type FC, type FormEvent, useEffect, useMemo, useState } from "react";
import TextareaAutosize from "react-textarea-autosize";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { cn } from "utils/cn";
import { ChatModelAdminPanel } from "./ChatModelAdminPanel/ChatModelAdminPanel";
import { SectionHeader } from "./SectionHeader";

type ConfigureAgentsSection =
	| "providers"
	| "system-prompt"
	| "models"
	| "user-prompt";

type ConfigureAgentsSectionOption = {
	id: ConfigureAgentsSection;
	label: string;
	icon: LucideIcon;
	adminOnly?: boolean;
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
	saveSystemPromptError: boolean;
	userPromptDraft: string;
	onUserPromptDraftChange: (value: string) => void;
	onSaveUserPrompt: (event: FormEvent) => void;
	isUserPromptDirty: boolean;
	saveUserPromptError: boolean;
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
	saveSystemPromptError,
	userPromptDraft,
	onUserPromptDraftChange,
	onSaveUserPrompt,
	isUserPromptDirty,
	saveUserPromptError,
	isDisabled,
}) => {
	const configureSectionOptions = useMemo<
		readonly ConfigureAgentsSectionOption[]
	>(() => {
		const options: ConfigureAgentsSectionOption[] = [];
		options.push({
			id: "user-prompt",
			label: "Custom Prompt",
			icon: MessageSquareTextIcon,
		});
		if (canManageChatModelConfigs) {
			options.push({
				id: "providers",
				label: "Providers",
				icon: KeyRoundIcon,
				adminOnly: true,
			});
			options.push({
				id: "models",
				label: "Models",
				icon: BoxesIcon,
				adminOnly: true,
			});
		}
		if (canSetSystemPrompt) {
			options.push({
				id: "system-prompt",
				label: "Behavior",
				icon: UserIcon,
				adminOnly: true,
			});
		}
		return options;
	}, [canManageChatModelConfigs, canSetSystemPrompt]);

	const [userActiveSection, setUserActiveSection] =
		useState<ConfigureAgentsSection>("user-prompt");

	const activeSection = configureSectionOptions.some(
		(s) => s.id === userActiveSection,
	)
		? userActiveSection
		: (configureSectionOptions[0]?.id ?? "user-prompt");

	useEffect(() => {
		if (open) {
			setUserActiveSection("user-prompt");
		}
	}, [open]);

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="grid h-[min(88dvh,720px)] max-w-4xl grid-cols-1 gap-0 overflow-hidden p-0 md:grid-cols-[220px_minmax(0,1fr)]">
				<DialogHeader className="sr-only">
					<DialogTitle>Settings</DialogTitle>
					<DialogDescription>
						Manage your personal preferences and agent configuration.
					</DialogDescription>
				</DialogHeader>

				<nav className="flex flex-row gap-0.5 overflow-x-auto border-b border-border bg-surface-secondary/40 p-2 md:flex-col md:gap-0.5 md:overflow-x-visible md:border-b-0 md:border-r md:p-4">
					<DialogClose asChild>
						<Button
							variant="subtle"
							size="icon-lg"
							className="mb-3 shrink-0 border-none bg-transparent shadow-none hover:bg-surface-tertiary/50"
						>
							<XIcon className="text-content-secondary" />
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
									"h-auto justify-start gap-3 rounded-lg border-none px-3 py-1.5 text-left shadow-none",
									isActive
										? "bg-surface-tertiary/60 text-content-primary hover:bg-surface-tertiary/60"
										: "bg-transparent text-content-secondary hover:bg-surface-tertiary/30 hover:text-content-primary",
								)}
								onClick={() => setUserActiveSection(section.id)}
							>
							<SectionIcon className="h-5 w-5 shrink-0" />
							<span className="flex items-center gap-2 text-sm font-medium">
								{section.label}
								{section.adminOnly && (
									<TooltipProvider>
										<Tooltip>
											<TooltipTrigger asChild>
												<ShieldIcon className="h-3 w-3 shrink-0 text-content-secondary" />
											</TooltipTrigger>
											<TooltipContent side="right">
												Admin only
											</TooltipContent>
										</Tooltip>
									</TooltipProvider>
								)}
							</span>							</Button>
						);
					})}
				</nav>

				<div className="flex min-h-0 flex-1 flex-col overflow-y-auto px-6 py-5">
					{activeSection === "user-prompt" && (
						<>
							<SectionHeader label="Custom Prompt" />
							<form
								className="space-y-4"
								onSubmit={(event) => void onSaveUserPrompt(event)}
							>
								<div className="space-y-2">
									<h3 className="m-0 text-[13px] font-semibold text-content-primary">
										Custom Prompt
									</h3>
									<p className="m-0 text-xs text-content-secondary">
										Personal instructions applied to all your new chats. This is
										only visible to you.
									</p>
									<TextareaAutosize
										className="min-h-[220px] w-full resize-y rounded-lg border border-border bg-surface-primary px-4 py-3 font-sans text-[13px] leading-relaxed text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-link/30"
										placeholder="Optional. Set personal instructions for your chats."
										value={userPromptDraft}
										onChange={(event) =>
											onUserPromptDraftChange(event.target.value)
										}
										disabled={isDisabled}
										minRows={7}
									/>
									<div className="flex justify-end gap-2">
										<Button
											size="sm"
											variant="outline"
											type="button"
											onClick={() => onUserPromptDraftChange("")}
											disabled={isDisabled || !userPromptDraft}
										>
											Clear
										</Button>
										<Button
											size="sm"
											type="submit"
											disabled={isDisabled || !isUserPromptDirty}
										>
											Save
										</Button>
									</div>
									{saveUserPromptError && (
										<p className="m-0 text-xs text-content-destructive">
											Failed to save custom prompt.
										</p>
									)}
								</div>
							</form>
						</>
					)}
					{activeSection === "providers" && canManageChatModelConfigs && (
						<ChatModelAdminPanel section="providers" sectionLabel="Providers" />
					)}
					{activeSection === "system-prompt" && canSetSystemPrompt && (
						<>
							<SectionHeader label="Behavior" />
							<form
								className="space-y-4"
								onSubmit={(event) => void onSaveSystemPrompt(event)}
							>
								<div className="space-y-2">
									<h3 className="m-0 text-[13px] font-semibold text-content-primary">
										System Prompt
									</h3>
									<p className="m-0 text-xs text-content-secondary">
										Admin-only instruction applied to all new chats. When empty,
										the built-in default prompt is used.
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
									{saveSystemPromptError && (
										<p className="m-0 text-xs text-content-destructive">
											Failed to save system prompt.
										</p>
									)}
								</div>
							</form>
						</>
					)}
					{activeSection === "models" && canManageChatModelConfigs && (
						<ChatModelAdminPanel section="models" sectionLabel="Models" />
					)}
				</div>
			</DialogContent>
		</Dialog>
	);
};
