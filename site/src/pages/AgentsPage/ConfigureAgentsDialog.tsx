import {
	chatSystemPrompt,
	chatUserCustomPrompt,
	updateChatSystemPrompt,
	updateUserChatCustomPrompt,
} from "api/queries/chats";
import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import type { LucideIcon } from "lucide-react";
import {
	BoxesIcon,
	KeyRoundIcon,
	ShieldIcon,
	UserIcon,
	XIcon,
} from "lucide-react";
import {
	type FC,
	type FormEvent,
	useCallback,
	useEffect,
	useMemo,
	useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import TextareaAutosize from "react-textarea-autosize";
import { cn } from "utils/cn";
import { ChatModelAdminPanel } from "./ChatModelAdminPanel/ChatModelAdminPanel";
import { SectionHeader } from "./SectionHeader";

type ConfigureAgentsSection = "providers" | "models" | "behavior";

type ConfigureAgentsSectionOption = {
	id: ConfigureAgentsSection;
	label: string;
	icon: LucideIcon;
	adminOnly?: boolean;
};

const AdminBadge: FC = () => (
	<TooltipProvider delayDuration={0}>
		<Tooltip>
			<TooltipTrigger asChild>
				<span className="inline-flex cursor-default items-center gap-1 rounded bg-surface-tertiary/60 px-1.5 py-px text-[11px] font-medium text-content-secondary">
					<ShieldIcon className="h-3 w-3" />
					Admin
				</span>
			</TooltipTrigger>
			<TooltipContent side="right">
				Only visible to deployment administrators.
			</TooltipContent>
		</Tooltip>
	</TooltipProvider>
);

const textareaClassName =
	"max-h-[240px] w-full resize-none overflow-y-auto rounded-lg border border-border bg-surface-primary px-4 py-3 font-sans text-[13px] leading-relaxed text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-link/30 [scrollbar-width:thin]";

interface ConfigureAgentsDialogProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	canManageChatModelConfigs: boolean;
	canSetSystemPrompt: boolean;
}

export const ConfigureAgentsDialog: FC<ConfigureAgentsDialogProps> = ({
	open,
	onOpenChange,
	canManageChatModelConfigs,
	canSetSystemPrompt,
}) => {
	const queryClient = useQueryClient();

	const systemPromptQuery = useQuery(chatSystemPrompt());
	const {
		mutate: saveSystemPrompt,
		isPending: isSavingSystemPrompt,
		isError: isSaveSystemPromptError,
	} = useMutation(updateChatSystemPrompt(queryClient));

	const userPromptQuery = useQuery(chatUserCustomPrompt());
	const {
		mutate: saveUserPrompt,
		isPending: isSavingUserPrompt,
		isError: isSaveUserPromptError,
	} = useMutation(updateUserChatCustomPrompt(queryClient));

	const serverPrompt = systemPromptQuery.data?.system_prompt ?? "";
	const [localEdit, setLocalEdit] = useState<string | null>(null);
	const systemPromptDraft = localEdit ?? serverPrompt;

	const serverUserPrompt = userPromptQuery.data?.custom_prompt ?? "";
	const [localUserEdit, setLocalUserEdit] = useState<string | null>(null);
	const userPromptDraft = localUserEdit ?? serverUserPrompt;

	const isSystemPromptDirty = localEdit !== null && localEdit !== serverPrompt;
	const isUserPromptDirty =
		localUserEdit !== null && localUserEdit !== serverUserPrompt;
	const isDisabled = isSavingSystemPrompt || isSavingUserPrompt;

	const handleSaveSystemPrompt = useCallback(
		(event: FormEvent) => {
			event.preventDefault();
			if (!isSystemPromptDirty) return;
			saveSystemPrompt(
				{ system_prompt: systemPromptDraft },
				{ onSuccess: () => setLocalEdit(null) },
			);
		},
		[isSystemPromptDirty, systemPromptDraft, saveSystemPrompt],
	);

	const handleSaveUserPrompt = useCallback(
		(event: FormEvent) => {
			event.preventDefault();
			if (!isUserPromptDirty) return;
			saveUserPrompt(
				{ custom_prompt: userPromptDraft },
				{ onSuccess: () => setLocalUserEdit(null) },
			);
		},
		[isUserPromptDirty, userPromptDraft, saveUserPrompt],
	);
	const configureSectionOptions = useMemo<
		readonly ConfigureAgentsSectionOption[]
	>(() => {
		const options: ConfigureAgentsSectionOption[] = [];
		options.push({
			id: "behavior",
			label: "Behavior",
			icon: UserIcon,
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
		return options;
	}, [canManageChatModelConfigs]);

	const [userActiveSection, setUserActiveSection] =
		useState<ConfigureAgentsSection>("behavior");

	const activeSection = configureSectionOptions.some(
		(s) => s.id === userActiveSection,
	)
		? userActiveSection
		: (configureSectionOptions[0]?.id ?? "behavior");

	useEffect(() => {
		if (open) {
			setUserActiveSection("behavior");
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
										<TooltipProvider delayDuration={0}>
											<Tooltip>
												<TooltipTrigger asChild>
													<span className="inline-flex">
														<ShieldIcon className="h-3 w-3 shrink-0 opacity-50" />
													</span>
												</TooltipTrigger>
												<TooltipContent side="right">Admin only</TooltipContent>
											</Tooltip>
										</TooltipProvider>
									)}
								</span>
							</Button>
						);
					})}
				</nav>

				<div className="flex min-h-0 flex-1 flex-col overflow-y-auto px-6 py-5 [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
					{activeSection === "behavior" && (
						<>
							<SectionHeader
								label="Behavior"
								description="Custom instructions that shape how the agent responds in your chats."
							/>
							{/* ── Personal prompt (always visible) ── */}
							<form
								className="space-y-2"
								onSubmit={(event) => void handleSaveUserPrompt(event)}
							>
								<h3 className="m-0 text-[13px] font-semibold text-content-primary">
									Personal Instructions{" "}
								</h3>
								<p className="!mt-0.5 m-0 text-xs text-content-secondary">
									Applied to all your chats. Only visible to you.
								</p>{" "}
								<TextareaAutosize
									className={textareaClassName}
									placeholder="Additional behavior, style, and tone preferences"
									value={userPromptDraft}
									onChange={(event) => setLocalUserEdit(event.target.value)}
									disabled={isDisabled}
									minRows={1}
								/>
								<div className="flex justify-end gap-2">
									<Button
										size="sm"
										variant="outline"
										type="button"
										onClick={() => setLocalUserEdit("")}
										disabled={isDisabled || !userPromptDraft}
									>
										Clear
									</Button>{" "}
									<Button
										size="sm"
										type="submit"
										disabled={isDisabled || !isUserPromptDirty}
									>
										Save
									</Button>
								</div>
								{isSaveUserPromptError && (
									<p className="m-0 text-xs text-content-destructive">
										Failed to save personal instructions.
									</p>
								)}
							</form>

							{/* ── Admin system prompt (admin only) ── */}
							{canSetSystemPrompt && (
								<>
									<hr className="my-5 border-0 border-t border-solid border-border" />
									<form
										className="space-y-2"
										onSubmit={(event) => void handleSaveSystemPrompt(event)}
									>
										<div className="flex items-center gap-2">
											<h3 className="m-0 text-[13px] font-semibold text-content-primary">
												System Instructions
											</h3>
											<AdminBadge />
										</div>
										<p className="!mt-0.5 m-0 text-xs text-content-secondary">
											Applied to all chats for every user. When empty, the
											built-in default is used.
										</p>{" "}
										<TextareaAutosize
											className={textareaClassName}
											placeholder="Additional behavior, style, and tone preferences for all users"
											value={systemPromptDraft}
											onChange={(event) => setLocalEdit(event.target.value)}
											disabled={isDisabled}
											minRows={1}
										/>
										<div className="flex justify-end gap-2">
											<Button
												size="sm"
												variant="outline"
												type="button"
												onClick={() => setLocalEdit("")}
												disabled={isDisabled || !systemPromptDraft}
											>
												Clear
											</Button>{" "}
											<Button
												size="sm"
												type="submit"
												disabled={isDisabled || !isSystemPromptDirty}
											>
												Save
											</Button>
										</div>
										{isSaveSystemPromptError && (
											<p className="m-0 text-xs text-content-destructive">
												Failed to save system prompt.
											</p>
										)}
									</form>
								</>
							)}
						</>
					)}
					{activeSection === "providers" && canManageChatModelConfigs && (
						<ChatModelAdminPanel
							section="providers"
							sectionLabel="Providers"
							sectionDescription="Connect third-party LLM services like OpenAI, Anthropic, or Google. Each provider supplies models that users can select for their chats."
							sectionBadge={<AdminBadge />}
						/>
					)}
					{activeSection === "models" && canManageChatModelConfigs && (
						<ChatModelAdminPanel
							section="models"
							sectionLabel="Models"
							sectionDescription="Choose which models from your configured providers are available for users to select. You can set a default and adjust context limits."
							sectionBadge={<AdminBadge />}
						/>
					)}
				</div>
			</DialogContent>
		</Dialog>
	);
};
