import { z } from "zod";

export const UserSchema = z.object({
	id: z.string().uuid(),
	username: z.string(),
	email: z.string().email(),
	organization_ids: z.array(z.string().uuid()),
	github_com_user_id: z.number().optional(),
});

export type User = z.infer<typeof UserSchema>;

export const UserListSchema = z.object({
	users: z.array(UserSchema),
});

export type UserList = z.infer<typeof UserListSchema>;

export const TaskSchema = z.object({
	id: z.string().uuid(),
	name: z.string(),
	owner_id: z.string().uuid(),
	template_id: z.string().uuid(),
	created_at: z.string(),
	updated_at: z.string(),
	status: z.string(),
});

export type Task = z.infer<typeof TaskSchema>;

export const TaskListSchema = z.object({
	tasks: z.array(TaskSchema),
});

export type TaskList = z.infer<typeof TaskListSchema>;

export const TemplateSchema = z.object({
	id: z.string().uuid(),
	name: z.string(),
	description: z.string().optional(),
	organization_id: z.string().uuid(),
	active_version_id: z.string().uuid(),
});

export type Template = z.infer<typeof TemplateSchema>;

export const ActionInputsSchema = z.object({
	coderUrl: z.string().url(),
	coderToken: z.string().min(1),
	templateName: z.string().min(1),
	taskPrompt: z.string().min(1),
	githubUserId: z.number().optional(),
	githubUsername: z.string().optional(),
	templatePreset: z.string().default("Default"),
	taskNamePrefix: z.string().default("task"),
	taskName: z.string().optional(),
	organization: z.string().default("coder"),
	issueUrl: z.string().url().optional(),
	commentOnIssue: z
		.union([z.boolean(), z.string()])
		.transform((val) => {
			if (typeof val === "boolean") return val;
			if (val === "false" || val === "0" || val === "") return false;
			return true;
		})
		.default(true),
	coderWebUrl: z.string().url().optional(),
	githubToken: z.string(),
});

export type ActionInputs = z.infer<typeof ActionInputsSchema>;

export const ActionOutputsSchema = z.object({
	coderUsername: z.string(),
	taskName: z.string(),
	taskUrl: z.string().url(),
	taskExists: z.boolean(),
});

export type ActionOutputs = z.infer<typeof ActionOutputsSchema>;

export const CreateTaskParamsSchema = z.object({
	name: z.string().min(1),
	owner: z.string().min(1),
	templateName: z.string().min(1),
	templatePreset: z.string().min(1),
	prompt: z.string().min(1),
	organization: z.string().min(1),
});

export type CreateTaskParams = z.infer<typeof CreateTaskParamsSchema>;
