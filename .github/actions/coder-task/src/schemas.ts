import { z } from "zod";

export type ActionInputs = z.infer<typeof ActionInputsSchema>;

export const ActionInputsSchema = z.object({
	coderUrl: z.string().url(),
	coderToken: z.string().min(1),
	templateName: z.string().min(1),
	taskPrompt: z.string().min(1),
	githubUserId: z.number().optional(),
	githubUsername: z.string().optional(),
	templatePreset: z.string(),
	taskNamePrefix: z.string(),
	taskName: z.string().optional(),
	organization: z.string(),
	issueUrl: z.string().url(),
	commentOnIssue: z
		.union([z.boolean(), z.string()])
		.transform((val) => {
			if (typeof val === "boolean") return val;
			if (val === "false" || val === "0" || val === "") return false;
			return true;
		})
		.default(true),
	githubToken: z.string(),
});

export const ActionOutputsSchema = z.object({
	coderUsername: z.string(),
	taskName: z.string(),
	taskUrl: z.string().url(),
	taskExists: z.boolean(),
});

export type ActionOutputs = z.infer<typeof ActionOutputsSchema>;
