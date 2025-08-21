import { API } from "api/api";
import type { Task } from "modules/tasks/tasks";

// TODO: This is a temporary solution while the BE does not return the Task in a
// right shape with a custom name. This should be removed once the BE is fixed.
export const data = {
	async createTask(
		prompt: string,
		userId: string,
		templateVersionId: string,
		presetId: string | undefined,
	): Promise<Task> {
		const workspace = await API.experimental.createTask(userId, {
			template_version_id: templateVersionId,
			template_version_preset_id: presetId,
			prompt,
		});

		return {
			workspace,
			prompt,
		};
	},
};
