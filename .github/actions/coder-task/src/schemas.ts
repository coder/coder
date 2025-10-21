import { z } from "zod";

export type ActionInputs = z.infer<typeof ActionInputsSchema>;

export const ActionInputsSchema = z.object({
	// Required
	coderTaskPrompt: z.string().min(1),
	coderToken: z.string().min(1),
	coderURL: z.string().url(),
	coderOrganization: z.string().min(1),
	coderTaskNamePrefix: z.string().min(1),
	coderTemplateName: z.string().min(1),
	githubIssueURL: z.string().url(),
	githubToken: z.string(),
	githubUserID: z.number().min(1),
	// Optional
	coderTemplatePreset: z.string().optional(),
});

export const ActionOutputsSchema = z.object({
	coderUsername: z.string(),
	taskName: z.string(),
	taskUrl: z.string().url(),
	taskExists: z.boolean(),
});

export type ActionOutputs = z.infer<typeof ActionOutputsSchema>;
