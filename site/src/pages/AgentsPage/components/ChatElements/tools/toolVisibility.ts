import { getSubagentChatId, getSubagentDescriptor } from "./subagentDescriptor";
import {
	asNumber,
	asRecord,
	asString,
	parseArgs,
	type ToolStatus,
	toProviderLabel,
} from "./utils";

type ExecuteRenderData = {
	command: string;
	output: string;
	durationMs?: number;
	isBackgrounded: boolean;
	authenticateURL: string;
	providerLabel: string;
};

export const getExecuteRenderData = (
	args: unknown,
	result: unknown,
): ExecuteRenderData => {
	const parsedArgs = parseArgs(args);
	const command = parsedArgs ? asString(parsedArgs.command) : "";
	const rec = asRecord(result);
	const output = rec ? asString(rec.output).trim() : "";
	const durationMs = rec
		? (asNumber(rec.wall_duration_ms, { parseString: true }) ??
			asNumber(rec.duration_ms, { parseString: true }))
		: undefined;
	const isBackgrounded = Boolean(
		rec && asString(rec.background_process_id).trim(),
	);
	const authenticateURL = rec?.auth_required
		? asString(rec.authenticate_url).trim()
		: "";
	const providerLabel = toProviderLabel(
		rec ? asString(rec.provider_display_name).trim() : "",
		rec ? asString(rec.provider_id).trim() : "",
		rec ? asString(rec.provider_type).trim() : "",
	);

	return {
		command,
		output,
		durationMs,
		isBackgrounded,
		authenticateURL,
		providerLabel,
	};
};

export const shouldHideExecuteTool = (data: ExecuteRenderData): boolean => {
	return data.command.trim().length === 0 && !data.authenticateURL;
};

export const shouldRenderTool = ({
	name,
	status,
	args,
	result,
}: {
	name: string;
	status: ToolStatus;
	args?: unknown;
	result?: unknown;
}): boolean => {
	if (name === "execute") {
		return !shouldHideExecuteTool(getExecuteRenderData(args, result));
	}

	const descriptor = getSubagentDescriptor({ name, args, result });
	if (!descriptor) {
		return true;
	}

	const chatId = getSubagentChatId({ args, result });
	if (
		!chatId &&
		status === "running" &&
		(descriptor.action === "wait" ||
			descriptor.action === "message" ||
			descriptor.action === "close")
	) {
		return false;
	}

	return true;
};
