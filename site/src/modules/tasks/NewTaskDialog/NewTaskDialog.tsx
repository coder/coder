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
import { type FC, useState, type FormEvent } from "react";
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
	const [freeFormPrompt, setFreeFormPrompt] = useState("");
	const [selectedSkill, setSelectedSkill] = useState<string>("");
	const [skillFollowUp, setSkillFollowUp] = useState("");
	const [controlLevel, setControlLevel] = useState(50);
	const [selectedAgent, setSelectedAgent] = useState("claude-code");
	const [showAdvanced, setShowAdvanced] = useState(false);
	const [selectedRepo, setSelectedRepo] = useState("");
	const [selectedTemplateId, setSelectedTemplateId] = useState<string>("");

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
			<DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
				<DialogHeader>
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

				<form onSubmit={handleSubmit} className="space-y-6">
					{/* Free-form Prompt Input - Only show when no skill selected */}
					{!selectedSkill && (
						<>
							<div className="space-y-2">
								<TextareaAutosize
									value={freeFormPrompt}
									onChange={(e) => setFreeFormPrompt(e.target.value)}
									placeholder="What would you like me to do?"
									className="w-full bg-surface-secondary border border-border border-solid rounded-lg p-3 outline-none resize-none text-sm placeholder:text-content-secondary focus:border-content-secondary"
									minRows={3}
									maxRows={8}
								/>
							</div>

							{/* Divider with "or" */}
							<div className="relative">
								<div className="absolute inset-0 flex items-center">
									<div className="w-full border-t border-border" />
								</div>
								<div className="relative flex justify-center text-xs uppercase">
									<span className="bg-surface-primary px-2 text-content-secondary">
										or
									</span>
								</div>
							</div>
						</>
					)}

					{/* Select a Skill Section */}
					<div className="space-y-3">
						<div className="flex items-center justify-between">
							<div>
								<h3 className="text-base font-semibold">Select a Skill</h3>
								<p className="text-xs text-content-secondary mt-0.5">
									Learn more? See{" "}
									<Link
										href="https://github.com/coder/coder/tree/main/.claude/skills"
										target="_blank"
										rel="noopener noreferrer"
										className="text-content-link hover:underline"
									>
										.claude/skills
									</Link>
								</p>
							</div>
							{selectedSkill && (
								<Button
									type="button"
									variant="subtle"
									size="sm"
									onClick={() => {
										setSelectedSkill("");
										setSkillFollowUp("");
									}}
								>
									Clear
								</Button>
							)}
						</div>
						<div className="flex flex-wrap gap-2">
							{SKILLS.map((skill) => (
								<button
									key={skill.id}
									type="button"
									onClick={() => {
										setSelectedSkill(skill.id);
										setSkillFollowUp("");
										setFreeFormPrompt(""); // Clear free-form when selecting skill
									}}
									className={cn(
										"px-4 py-2 rounded-lg text-sm font-medium transition-all border border-solid",
										selectedSkill === skill.id
											? "bg-content-primary text-surface-primary border-content-primary"
											: "bg-surface-secondary text-content-secondary border-border hover:border-content-secondary",
									)}
								>
									{skill.label}
								</button>
							))}
						</div>
					</div>

					{/* Follow-up Input - Shows when skill is selected */}
					{selectedSkill && (
						<div className="space-y-2">
							<label className="text-sm font-medium">
								{SKILLS.find((s) => s.id === selectedSkill)?.followUpPrompt}
							</label>
							<TextareaAutosize
								value={skillFollowUp}
								onChange={(e) => setSkillFollowUp(e.target.value)}
								placeholder="Provide details..."
								className="w-full bg-surface-secondary border border-border border-solid rounded-lg p-3 outline-none resize-none text-sm placeholder:text-content-secondary focus:border-content-secondary"
								minRows={3}
								maxRows={8}
							/>
						</div>
					)}

					{/* Agent Picker - Clean Pills */}
					<div className="space-y-2">
						<label className="text-sm font-medium">Agent</label>
						<div className="flex gap-2">
							{AGENTS.map((agent) => (
								<button
									key={agent.id}
									type="button"
									onClick={() => setSelectedAgent(agent.id)}
									className={cn(
										"flex items-center gap-2 px-4 py-2.5 rounded-lg border border-solid transition-all",
										selectedAgent === agent.id
											? "bg-content-primary text-surface-primary border-content-primary"
											: "bg-surface-secondary text-content-secondary border-border hover:border-content-secondary",
									)}
								>
									<img src={agent.icon} alt={agent.label} className="size-5" />
									<span className="font-medium">{agent.label}</span>
								</button>
							))}
						</div>
					</div>

					{/* Level of Control Slider */}
					<div className="space-y-2">
						<div className="flex justify-between items-center">
							<label className="text-sm font-medium">Level of Control</label>
							<span className="text-xs text-content-secondary">
								{controlLevel}% {controlLevel < 50 ? "Autonomous" : "Guided"}
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
						<div className="flex justify-between text-xs text-content-secondary">
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

					{/* Create Button */}
					<div className="flex justify-end gap-2 pt-4 border-t border-border">
						<Button
							type="button"
							variant="outline"
							onClick={onClose}
							disabled={createTaskMutation.isPending}
						>
							Cancel
						</Button>
						<Button type="submit" disabled={!canSubmit}>
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
				</form>
			</DialogContent>
		</Dialog>
	);
};
