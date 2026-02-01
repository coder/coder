import { API } from "api/api";
import { getErrorMessage } from "api/errors";
import { templates } from "api/queries/templates";
import type { Template } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { Slider } from "components/Slider/Slider";
import { Spinner } from "components/Spinner/Spinner";
import { displaySuccess, displayError } from "components/GlobalSnackbar/utils";
import { useAuthenticated } from "hooks/useAuthenticated";
import { Link } from "components/Link/Link";
import { SettingsIcon } from "lucide-react";
import { type FC, useState, type FormEvent, useEffect, useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import TextareaAutosize from "react-textarea-autosize";
import { useNavigate } from "react-router";
import { cn } from "utils/cn";

type NewTaskDialogProps = {
	open: boolean;
	onClose: () => void;
};

type Skill = {
	id: string;
	label: string;
	followUpPrompt: string;
	subtext?: string;
};

const SKILLS: Skill[] = [
	{
		id: "code-review",
		label: "Code Review",
		followUpPrompt: "What aspects should I focus on?",
	},
	{
		id: "debugging",
		label: "Debugging",
		followUpPrompt: "What's the issue you're experiencing?",
	},
	{
		id: "refactoring",
		label: "Refactoring",
		followUpPrompt: "What needs to be refactored?",
	},
	{
		id: "testing",
		label: "Testing",
		followUpPrompt: "What needs test coverage?",
	},
	{
		id: "documentation",
		label: "Documentation",
		followUpPrompt: "What needs documentation?",
	},
];

const AGENTS = [
	{ id: "mux", label: "Mux", icon: "/icon/coder.svg" },
	{ id: "claude-code", label: "Claude Code", icon: "/icon/claude.svg" },
];

export const NewTaskDialog: FC<NewTaskDialogProps> = ({ open, onClose }) => {
	const { user } = useAuthenticated();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const freeFormRef = useRef<HTMLTextAreaElement>(null);
	const followUpRef = useRef<HTMLTextAreaElement>(null);
	const [freeFormPrompt, setFreeFormPrompt] = useState("");
	const [selectedSkill, setSelectedSkill] = useState<string>("");
	const [skillFollowUp, setSkillFollowUp] = useState("");
	const [controlLevel, setControlLevel] = useState(50);
	const [selectedAgent, setSelectedAgent] = useState("claude-code");
	const [showAdvanced, setShowAdvanced] = useState(false);
	const [selectedRepo, setSelectedRepo] = useState("");
	const [selectedTemplateId, setSelectedTemplateId] = useState<string>("");

	// Auto-focus the appropriate input
	useEffect(() => {
		if (!open) return;

		// Small delay to ensure dialog is rendered
		const timer = setTimeout(() => {
			if (selectedSkill && followUpRef.current) {
				followUpRef.current.focus();
			} else if (!selectedSkill && freeFormRef.current) {
				freeFormRef.current.focus();
			}
		}, 100);

		return () => clearTimeout(timer);
	}, [open, selectedSkill]);

	// Handle Cmd+Enter to close modal (prototyping)
	useEffect(() => {
		const handleKeyDown = (e: KeyboardEvent) => {
			if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
				e.preventDefault();
				onClose();
			}
		};

		if (open) {
			window.addEventListener("keydown", handleKeyDown);
			return () => window.removeEventListener("keydown", handleKeyDown);
		}
	}, [open, onClose]);

	const aiTemplatesQuery = useQuery(
		templates({
			q: "has-ai-task:true",
		}),
	);

	// Set default template when templates load
	const selectedTemplate = aiTemplatesQuery.data?.find(
		(t) =>
			t.id === selectedTemplateId ||
			(!selectedTemplateId && aiTemplatesQuery.data),
	);

	// Generate the full prompt
	const getFullPrompt = () => {
		// If user typed a free-form prompt, use that
		if (freeFormPrompt.trim()) {
			return freeFormPrompt;
		}

		// Otherwise use skill-based prompt
		if (!selectedSkill) return "";

		const skill = SKILLS.find((s) => s.id === selectedSkill);
		if (!skill) return "";

		let prompt = `${skill.label}`;
		if (skillFollowUp.trim()) {
			prompt += `: ${skillFollowUp}`;
		}

		return prompt;
	};

	const createTaskMutation = useMutation({
		mutationFn: async () => {
			if (!selectedTemplate) throw new Error("No template selected");

			const fullPrompt = getFullPrompt();
			if (!fullPrompt)
				throw new Error("Please select a skill and provide details");

			const task = await API.createTask(user.id, {
				input: fullPrompt,
				template_version_id: selectedTemplate.active_version_id,
			});
			return task;
		},
		onSuccess: async (task) => {
			await queryClient.invalidateQueries({ queryKey: ["tasks"] });
			displaySuccess("Task created successfully");
			navigate(`/tasks/${task.owner_name}/${task.id}`);
			onClose();
			setFreeFormPrompt("");
			setSelectedSkill("");
			setSkillFollowUp("");
			setControlLevel(50);
			setSelectedAgent("claude-code");
			setSelectedRepo("");
		},
		onError: (error) => {
			displayError(getErrorMessage(error, "Failed to create task"));
		},
	});

	const handleSubmit = (e: FormEvent) => {
		e.preventDefault();
		const fullPrompt = getFullPrompt();
		if (fullPrompt.trim()) {
			createTaskMutation.mutate();
		}
	};

	const canSubmit =
		getFullPrompt().trim().length > 0 && !createTaskMutation.isPending;

	return (
		<Dialog open={open} onOpenChange={onClose}>
			<DialogContent className="max-w-4xl max-h-[90vh] overflow-hidden p-0">
				<div className="flex h-full max-h-[90vh]">
					{/* Left Side - Main Form */}
					<div className="flex-1 flex flex-col overflow-hidden">
						<DialogHeader className="px-6 py-4 border-b border-border">
							<div className="flex items-center justify-between">
								<DialogTitle>New Task</DialogTitle>
								<Button
									variant="subtle"
									size="sm"
									onClick={() => setShowAdvanced(!showAdvanced)}
								>
									<SettingsIcon className="size-4" />
									{showAdvanced ? "Hide" : "Show"} Advanced
								</Button>
							</div>
						</DialogHeader>

						<form
							onSubmit={handleSubmit}
							className="flex-1 overflow-y-auto px-6 py-4 space-y-4"
						>
							{/* Dynamic prompt based on selection */}
							{!selectedSkill ? (
								<div>
									<label className="text-xs font-medium text-content-secondary block mb-1.5">
										What would you like me to do?
									</label>
									<TextareaAutosize
										ref={freeFormRef}
										value={freeFormPrompt}
										onChange={(e) => setFreeFormPrompt(e.target.value)}
										placeholder="Describe your task..."
										className="w-full bg-surface-secondary border border-border border-solid rounded-lg p-3 outline-none resize-none text-sm placeholder:text-content-secondary focus:border-content-link focus:ring-1 focus:ring-content-link"
										minRows={3}
										maxRows={8}
									/>
								</div>
							) : (
								<div>
									<label className="text-xs font-medium text-content-secondary block mb-1.5">
										{SKILLS.find((s) => s.id === selectedSkill)?.followUpPrompt}
									</label>
									<TextareaAutosize
										ref={followUpRef}
										value={skillFollowUp}
										onChange={(e) => setSkillFollowUp(e.target.value)}
										placeholder="Provide details..."
										className="w-full bg-surface-secondary border border-border border-solid rounded-lg p-3 outline-none resize-none text-sm placeholder:text-content-secondary focus:border-content-link focus:ring-1 focus:ring-content-link"
										minRows={3}
										maxRows={8}
									/>
								</div>
							)}

							{/* Agent Picker - Compact */}
							<div className="space-y-1.5">
								<label className="text-xs font-medium text-content-secondary">
									Agent
								</label>
								<div className="flex gap-2">
									{AGENTS.map((agent) => (
										<button
											key={agent.id}
											type="button"
											onClick={() => setSelectedAgent(agent.id)}
											className={cn(
												"flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium border border-solid transition-all",
												selectedAgent === agent.id
													? "bg-content-primary text-surface-primary border-content-primary"
													: "bg-surface-secondary text-content-secondary border-border hover:border-content-secondary hover:bg-surface-invert-secondary",
											)}
										>
											<img
												src={agent.icon}
												alt={agent.label}
												className="size-4"
											/>
											<span>{agent.label}</span>
										</button>
									))}
								</div>
							</div>

							{/* Level of Control Slider - Compact */}
							<div className="space-y-1.5">
								<div className="flex justify-between items-center">
									<label className="text-xs font-medium text-content-secondary">
										Level of Control
									</label>
									<span className="text-xs text-content-secondary">
										{controlLevel}%
									</span>
								</div>
								<Slider
									value={[controlLevel]}
									onValueChange={(value) => setControlLevel(value[0])}
									min={0}
									max={100}
									step={10}
									className="w-full"
								/>
								<div className="flex justify-between text-[10px] text-content-secondary">
									<span>Autonomous</span>
									<span>Guided</span>
								</div>
							</div>

							{/* Advanced Section */}
							{showAdvanced && (
								<div className="space-y-4 pt-4 border-t border-border">
									<h3 className="text-sm font-medium">Advanced Options</h3>

									{/* Repository Picker */}
									<div className="space-y-2">
										<label className="text-sm font-medium">Repository</label>
										<input
											type="text"
											value={selectedRepo}
											onChange={(e) => setSelectedRepo(e.target.value)}
											placeholder="owner/repo"
											className="w-full bg-surface-secondary border border-border border-solid rounded-lg px-3 py-2 text-sm outline-none focus:border-content-secondary"
										/>
									</div>

									{/* Template Version Picker */}
									<div className="space-y-2">
										<label className="text-sm font-medium">Template</label>
										<Select
											value={selectedTemplateId}
											onValueChange={setSelectedTemplateId}
										>
											<SelectTrigger className="w-full">
												<SelectValue placeholder="Use default template" />
											</SelectTrigger>
											<SelectContent>
												{aiTemplatesQuery.data?.map((template) => (
													<SelectItem key={template.id} value={template.id}>
														{template.display_name || template.name}
													</SelectItem>
												))}
											</SelectContent>
										</Select>
									</div>
								</div>
							)}
						</form>

						{/* Create Button - Footer */}
						<div className="px-6 py-3 border-t border-border flex justify-end gap-2">
							<Button
								type="button"
								variant="outline"
								size="sm"
								onClick={onClose}
								disabled={createTaskMutation.isPending}
							>
								Cancel
							</Button>
							<Button
								type="submit"
								size="sm"
								disabled={!canSubmit}
								onClick={handleSubmit}
							>
								{createTaskMutation.isPending ? (
									<>
										<Spinner />
										Creating...
									</>
								) : (
									"Create Task"
								)}
							</Button>
						</div>
					</div>

					{/* Right Sidebar - Skills */}
					<div className="w-64 bg-surface-secondary border-l border-border overflow-y-auto">
						<div className="p-4 border-b border-border">
							<h3 className="text-sm font-semibold mb-1">Select a Skill</h3>
							<p className="text-xs text-content-secondary">
								See your organization's{" "}
								<Link
									href="https://github.com/coder/coder/tree/main/.agent/skills"
									target="_blank"
									rel="noopener noreferrer"
									className="text-content-link hover:underline"
								>
									.agent/skills
								</Link>
							</p>
						</div>
						<div className="p-3 space-y-1.5">
							{/* Free-form option as first skill */}
							<button
								type="button"
								onClick={() => {
									setSelectedSkill("");
									setSkillFollowUp("");
								}}
								className={cn(
									"w-full text-left px-3 py-2 rounded-md text-sm font-medium transition-all",
									!selectedSkill
										? "bg-content-primary text-surface-primary"
										: "text-content-secondary hover:bg-surface-invert-secondary hover:text-content-primary",
								)}
							>
								Free-form Prompt
							</button>
							{SKILLS.map((skill) => (
								<button
									key={skill.id}
									type="button"
									onClick={() => {
										setSelectedSkill(skill.id);
										setSkillFollowUp("");
										setFreeFormPrompt("");
									}}
									className={cn(
										"w-full text-left px-3 py-2 rounded-md text-sm font-medium transition-all",
										selectedSkill === skill.id
											? "bg-content-primary text-surface-primary"
											: "text-content-secondary hover:bg-surface-invert-secondary hover:text-content-primary",
									)}
								>
									{skill.label}
								</button>
							))}
						</div>
					</div>
				</div>
			</DialogContent>
		</Dialog>
	);
};
