import { API } from "api/api";
import { getErrorMessage } from "api/errors";
import { templates } from "api/queries/templates";
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
import { Switch } from "components/Switch/Switch";
import { displaySuccess, displayError } from "components/GlobalSnackbar/utils";
import { useAuthenticated } from "hooks/useAuthenticated";
import { Link } from "components/Link/Link";
import { SettingsIcon, Code2, InfoIcon } from "lucide-react";
import { type FC, useState, type FormEvent, useEffect, useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import TextareaAutosize from "react-textarea-autosize";
import { useNavigate, Link as RouterLink } from "react-router";
import { cn } from "utils/cn";

type DuplicateMetadata = {
	workspaceName: string;
	workspaceOwner: string;
	branch: string;
	repository: string;
	pvcName: string;
};

type NewTaskDialogProps = {
	open: boolean;
	onClose: () => void;
	duplicateMetadata?: DuplicateMetadata;
};

type SkillInput = {
	type: "text" | "textarea" | "multiselect" | "toggle" | "select";
	label: string;
	placeholder?: string;
	options?: string[];
	key: string;
};

type Skill = {
	id: string;
	label: string;
	followUpPrompt: string;
	inputs: SkillInput[];
	subtext?: string;
};

const SKILLS: Skill[] = [
	{
		id: "code-review",
		label: "Code Review",
		followUpPrompt: "What aspects should I focus on?",
		inputs: [
			{
				type: "textarea",
				label: "What code needs review?",
				placeholder: "Specify files, functions, or areas...",
				key: "code_location",
			},
			{
				type: "multiselect",
				label: "Focus areas",
				options: [
					"Security",
					"Performance",
					"Best Practices",
					"Readability",
					"Testing",
				],
				key: "focus_areas",
			},
			{
				type: "toggle",
				label: "Include suggestions for improvements",
				key: "include_suggestions",
			},
		],
	},
	{
		id: "debugging",
		label: "Debugging",
		followUpPrompt: "What's the issue you're experiencing?",
		inputs: [
			{
				type: "textarea",
				label: "Describe the issue",
				placeholder: "What's happening vs what should happen?",
				key: "issue_description",
			},
			{
				type: "text",
				label: "Error message (if any)",
				placeholder: "Copy the error message here...",
				key: "error_message",
			},
			{
				type: "select",
				label: "When does it occur?",
				options: [
					"Always",
					"Intermittently",
					"First time only",
					"After changes",
				],
				key: "occurrence",
			},
		],
	},
	{
		id: "refactoring",
		label: "Refactoring",
		followUpPrompt: "What needs to be refactored?",
		inputs: [
			{
				type: "textarea",
				label: "What needs refactoring?",
				placeholder: "Files, functions, or components...",
				key: "refactor_target",
			},
			{
				type: "multiselect",
				label: "Goals",
				options: [
					"Simplify logic",
					"Improve performance",
					"Extract reusable code",
					"Update patterns",
					"Reduce duplication",
				],
				key: "refactor_goals",
			},
		],
	},
	{
		id: "testing",
		label: "Testing",
		followUpPrompt: "What needs test coverage?",
		inputs: [
			{
				type: "textarea",
				label: "What needs testing?",
				placeholder: "Files, functions, or features...",
				key: "test_target",
			},
			{
				type: "multiselect",
				label: "Test types",
				options: ["Unit tests", "Integration tests", "E2E tests", "Edge cases"],
				key: "test_types",
			},
			{
				type: "toggle",
				label: "Generate test fixtures/mocks",
				key: "generate_fixtures",
			},
		],
	},
	{
		id: "documentation",
		label: "Documentation",
		followUpPrompt: "What needs documentation?",
		inputs: [
			{
				type: "textarea",
				label: "What needs documentation?",
				placeholder: "APIs, functions, components, or features...",
				key: "doc_target",
			},
			{
				type: "select",
				label: "Documentation type",
				options: [
					"API reference",
					"User guide",
					"Code comments",
					"README",
					"Architecture docs",
				],
				key: "doc_type",
			},
			{
				type: "toggle",
				label: "Include examples",
				key: "include_examples",
			},
		],
	},
];

const AGENTS = [
	{ id: "mux", label: "Mux", icon: "/icon/coder.svg" },
	{ id: "claude-code", label: "Claude Code", icon: "/icon/claude.svg" },
];

export const NewTaskDialog: FC<NewTaskDialogProps> = ({
	open,
	onClose,
	duplicateMetadata,
}) => {
	const { user } = useAuthenticated();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const freeFormRef = useRef<HTMLTextAreaElement>(null);
	const [freeFormPrompt, setFreeFormPrompt] = useState("");
	const [selectedSkill, setSelectedSkill] = useState<string>("");
	const [skillInputs, setSkillInputs] = useState<Record<string, any>>({});
	const [controlLevel, setControlLevel] = useState(50);
	const [selectedAgent, setSelectedAgent] = useState("claude-code");
	const [showAdvanced, setShowAdvanced] = useState(false);
	const [selectedRepo, setSelectedRepo] = useState("");
	const [selectedTemplateId, setSelectedTemplateId] = useState<string>("");
	const [showApiCode, setShowApiCode] = useState(false);
	const [apiCodeType, setApiCodeType] = useState<
		"curl" | "cli" | "sdk" | "github"
	>("curl");

	// Auto-focus the free-form input
	useEffect(() => {
		if (!open) return;

		const timer = setTimeout(() => {
			if (freeFormRef.current) {
				freeFormRef.current.focus();
			}
		}, 100);

		return () => clearTimeout(timer);
	}, [open]);

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

		// Otherwise use skill-based prompt with inputs
		if (!selectedSkill) return "";

		const skill = SKILLS.find((s) => s.id === selectedSkill);
		if (!skill) return "";

		let prompt = `${skill.label}:\n\n`;

		// Add all filled inputs to the prompt
		skill.inputs.forEach((input) => {
			const value = skillInputs[input.key];
			if (!value) return;

			if (input.type === "multiselect" && Array.isArray(value)) {
				if (value.length > 0) {
					prompt += `${input.label}: ${value.join(", ")}\n`;
				}
			} else if (input.type === "toggle") {
				if (value === true) {
					prompt += `${input.label}: Yes\n`;
				}
			} else if (value) {
				prompt += `${input.label}: ${value}\n`;
			}
		});

		return prompt.trim();
	};

	const createTaskMutation = useMutation({
		mutationFn: async () => {
			if (!selectedTemplate) throw new Error("No template selected");

			const fullPrompt = getFullPrompt();
			if (!fullPrompt) throw new Error("Please provide task details");

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
			setSkillInputs({});
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

	const selectedSkillData = SKILLS.find((s) => s.id === selectedSkill);

	// Generate API code examples
	const getApiCode = () => {
		const prompt = getFullPrompt() || "Your task description here";
		const templateId =
			selectedTemplate?.active_version_id || "template_version_id";

		if (apiCodeType === "curl") {
			return `curl -X POST "$CODER_URL/api/v2/users/me/tasks" \\
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "input": ${JSON.stringify(prompt)},
    "template_version_id": "${templateId}"
  }'`;
		}

		if (apiCodeType === "cli") {
			return `coder tasks create \\
  --input ${JSON.stringify(prompt)} \\
  --template-version ${templateId}`;
		}

		if (apiCodeType === "github") {
			return `name: Create Coder Task

on:
  push:
    branches: [main]
  workflow_dispatch:

jobs:
  create-task:
    runs-on: ubuntu-latest
    steps:
      - name: Create Coder Task
        run: |
          curl -X POST "$CODER_URL/api/v2/users/me/tasks" \\
            -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \\
            -H "Content-Type: application/json" \\
            -d '{
              "input": ${JSON.stringify(prompt)},
              "template_version_id": "${templateId}"
            }'
        env:
          CODER_URL: \${{ secrets.CODER_URL }}
          CODER_SESSION_TOKEN: \${{ secrets.CODER_SESSION_TOKEN }}`;
		}

		// SDK (TypeScript)
		return `import { API } from "@coder/api";

const task = await API.createTask(userId, {
  input: ${JSON.stringify(prompt)},
  template_version_id: "${templateId}",
});

console.log(\`Task created: \${task.id}\`);`;
	};

	return (
		<Dialog open={open} onOpenChange={onClose}>
			<DialogContent className="max-w-4xl max-h-[90vh] overflow-hidden p-0">
				<div className="flex h-full max-h-[90vh]">
					{/* Left Side - Main Form */}
					<div className="flex-1 flex flex-col overflow-hidden">
						<DialogHeader className="px-6 py-4 border-b border-border">
							<div className="flex items-center justify-between">
								<DialogTitle>New Task</DialogTitle>
								<div className="flex items-center gap-2">
									{showApiCode ? (
										<Select
											value={apiCodeType}
											onValueChange={(value) =>
												setApiCodeType(
													value as "curl" | "cli" | "sdk" | "github",
												)
											}
										>
											<SelectTrigger className="w-[180px]">
												<Code2 className="size-4" />
												<SelectValue />
											</SelectTrigger>
											<SelectContent>
												<SelectItem value="curl">curl</SelectItem>
												<SelectItem value="cli">CLI</SelectItem>
												<SelectItem value="sdk">SDK</SelectItem>
												<SelectItem value="github">GitHub Actions</SelectItem>
											</SelectContent>
										</Select>
									) : (
										<Button
											variant="subtle"
											size="sm"
											onClick={() => setShowAdvanced(!showAdvanced)}
										>
											<SettingsIcon className="size-4" />
											{showAdvanced ? "Hide" : "Show"} Advanced
										</Button>
									)}
									<Button
										variant="subtle"
										size="sm"
										onClick={() => {
											setShowApiCode(!showApiCode);
											setSelectedSkill("");
											setSkillInputs({});
										}}
									>
										<Code2 className="size-4" />
										API
									</Button>
								</div>
							</div>
						</DialogHeader>

						{/* Duplicate metadata banner */}
						{duplicateMetadata && (
							<div className="mx-6 mt-4 p-4 bg-surface-secondary border-2 border-content-link/20 rounded-lg">
								<div className="flex items-start gap-3">
									<InfoIcon className="size-5 text-content-link flex-shrink-0 mt-0.5" />
									<div className="flex-1 space-y-3">
										<p className="m-0 text-sm font-semibold text-content-primary">
											Duplicating from{" "}
											<span className="font-mono text-content-link">
												{duplicateMetadata.workspaceName}
											</span>
										</p>
										<div className="grid grid-cols-3 gap-4 pt-2 border-t border-border">
											<div>
												<p className="m-0 font-semibold text-xs text-content-secondary mb-1.5">
													Branch
												</p>
												<p className="m-0 font-mono text-sm text-content-primary">
													{duplicateMetadata.branch}
												</p>
											</div>
											<div>
												<p className="m-0 font-semibold text-xs text-content-secondary mb-1.5">
													Repository
												</p>
												<p className="m-0 font-mono text-sm text-content-primary">
													{duplicateMetadata.repository}
												</p>
											</div>
											<div>
												<p className="m-0 font-semibold text-xs text-content-secondary mb-1.5">
													PersistentVolumeClaim
												</p>
												<p className="m-0 font-mono text-sm text-content-primary">
													{duplicateMetadata.pvcName}
												</p>
											</div>
										</div>
										<div className="pt-3 border-t border-border mt-3">
											<Button
												variant="subtle"
												size="sm"
												className="p-0 min-w-0 text-xs"
												asChild
											>
												<RouterLink
													to={`/@${duplicateMetadata.workspaceOwner}/${duplicateMetadata.workspaceName}/terminal`}
												>
													View conversation history →
												</RouterLink>
											</Button>
										</div>
									</div>
								</div>
							</div>
						)}

						<form
							onSubmit={handleSubmit}
							className="flex-1 overflow-y-auto px-6 py-4 space-y-4"
						>
							{/* Show API code example when API mode is active */}
							{showApiCode && (
								<div className="space-y-2">
									<pre className="bg-surface-secondary border border-border rounded-lg p-4 text-xs overflow-x-auto">
										<code className="text-content-secondary font-mono whitespace-pre">
											{getApiCode()}
										</code>
									</pre>
									<p className="text-xs text-content-secondary">
										Use this to automate task creation via API
									</p>
								</div>
							)}

							{/* Free-form prompt input - show when not in API mode */}
							{!showApiCode && !selectedSkillData && (
								<div>
									<label className="text-xs font-medium text-content-secondary block mb-1.5">
										What would you like me to do?
									</label>
									<TextareaAutosize
										ref={freeFormRef}
										value={freeFormPrompt}
										onChange={(e) => {
											setFreeFormPrompt(e.target.value);
											// Clear skill selection when typing
											if (e.target.value.trim() && selectedSkill) {
												setSelectedSkill("");
												setSkillInputs({});
											}
										}}
										placeholder="Describe your task..."
										className="w-full bg-surface-secondary border border-border border-solid rounded-lg p-3 outline-none resize-none text-sm placeholder:text-content-secondary focus:border-content-link focus:ring-1 focus:ring-content-link"
										minRows={3}
										maxRows={8}
									/>
								</div>
							)}

							{/* Skill-specific inputs */}
							{!showApiCode && selectedSkillData && (
								<div className="space-y-4">
									{selectedSkillData.inputs.map((input) => (
										<div key={input.key}>
											{input.type === "textarea" && (
												<div>
													<label className="text-xs font-medium text-content-secondary block mb-1.5">
														{input.label}
													</label>
													<TextareaAutosize
														value={skillInputs[input.key] || ""}
														onChange={(e) =>
															setSkillInputs((prev) => ({
																...prev,
																[input.key]: e.target.value,
															}))
														}
														placeholder={input.placeholder}
														className="w-full bg-surface-secondary border border-border border-solid rounded-lg p-3 outline-none resize-none text-sm placeholder:text-content-secondary focus:border-content-link focus:ring-1 focus:ring-content-link"
														minRows={2}
														maxRows={6}
													/>
												</div>
											)}

											{input.type === "text" && (
												<div>
													<label className="text-xs font-medium text-content-secondary block mb-1.5">
														{input.label}
													</label>
													<input
														type="text"
														value={skillInputs[input.key] || ""}
														onChange={(e) =>
															setSkillInputs((prev) => ({
																...prev,
																[input.key]: e.target.value,
															}))
														}
														placeholder={input.placeholder}
														className="w-full bg-surface-secondary border border-border border-solid rounded-lg px-3 py-2 text-sm outline-none focus:border-content-link focus:ring-1 focus:ring-content-link"
													/>
												</div>
											)}

											{input.type === "select" && input.options && (
												<div>
													<label className="text-xs font-medium text-content-secondary block mb-1.5">
														{input.label}
													</label>
													<Select
														value={skillInputs[input.key] || ""}
														onValueChange={(value) =>
															setSkillInputs((prev) => ({
																...prev,
																[input.key]: value,
															}))
														}
													>
														<SelectTrigger className="w-full">
															<SelectValue placeholder="Select an option..." />
														</SelectTrigger>
														<SelectContent>
															{input.options.map((option) => (
																<SelectItem key={option} value={option}>
																	{option}
																</SelectItem>
															))}
														</SelectContent>
													</Select>
												</div>
											)}

											{input.type === "multiselect" && input.options && (
												<div>
													<label className="text-xs font-medium text-content-secondary block mb-1.5">
														{input.label}
													</label>
													<div className="flex flex-wrap gap-2">
														{input.options.map((option) => {
															const selected = (
																skillInputs[input.key] || []
															).includes(option);
															return (
																<button
																	key={option}
																	type="button"
																	onClick={() => {
																		const current =
																			skillInputs[input.key] || [];
																		const updated = selected
																			? current.filter(
																					(v: string) => v !== option,
																				)
																			: [...current, option];
																		setSkillInputs((prev) => ({
																			...prev,
																			[input.key]: updated,
																		}));
																	}}
																	className={cn(
																		"px-2.5 py-1 rounded-md text-xs font-medium transition-all border border-solid",
																		selected
																			? "bg-content-primary text-surface-primary border-content-primary"
																			: "bg-surface-secondary text-content-secondary border-border hover:border-content-secondary hover:bg-surface-invert-secondary",
																	)}
																>
																	{option}
																</button>
															);
														})}
													</div>
												</div>
											)}

											{input.type === "toggle" && (
												<div className="flex items-center justify-between">
													<label className="text-xs font-medium text-content-secondary">
														{input.label}
													</label>
													<Switch
														checked={skillInputs[input.key] || false}
														onCheckedChange={(checked) =>
															setSkillInputs((prev) => ({
																...prev,
																[input.key]: checked,
															}))
														}
													/>
												</div>
											)}
										</div>
									))}
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
					<div className="w-64 bg-surface-secondary border-l border-border overflow-y-auto flex flex-col">
						{/* Free Form Task button */}
						<div className="p-3 pb-2 space-y-2">
							<button
								type="button"
								onClick={() => {
									setSelectedSkill("");
									setSkillInputs({});
									setShowApiCode(false);
									// Focus the main textarea
									setTimeout(() => {
										if (freeFormRef.current) {
											freeFormRef.current.focus();
										}
									}, 50);
								}}
								className={cn(
									"w-full text-left px-3 py-2 rounded-md text-sm font-medium transition-all",
									!selectedSkill && !showApiCode
										? "bg-content-primary text-surface-primary"
										: "text-content-secondary hover:bg-surface-invert-secondary hover:text-content-primary",
								)}
							>
								Free Form Task
							</button>

							<h3 className="text-sm font-semibold px-1 pt-1">
								or select a skill:
							</h3>
						</div>

						{/* Skills section */}
						<div className="flex-1 overflow-y-auto">
							<div className="px-3 space-y-1.5">
								{SKILLS.map((skill) => (
									<button
										key={skill.id}
										type="button"
										onClick={() => {
											setSelectedSkill(skill.id);
											setSkillInputs({});
											setFreeFormPrompt(""); // Clear free-form when selecting skill
											setShowApiCode(false);
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

						{/* Footer text */}
						<div className="p-3 border-t border-border">
							<p className="text-xs text-content-secondary">
								From your{" "}
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
					</div>
				</div>
			</DialogContent>
		</Dialog>
	);
};
