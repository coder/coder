import {
	SCENARIO_AGENT_CRASH,
	SCENARIO_DERP_FALLBACK,
	SCENARIO_DEVICE_SLEEP,
	SCENARIO_WORKSPACE_STOP,
} from "./mockData";
import type { UserDiagnosticResponse } from "./types";

const SCENARIO_MAP: Record<string, UserDiagnosticResponse> = {
	"sarah-chen": SCENARIO_DEVICE_SLEEP,
	"john-ops": SCENARIO_WORKSPACE_STOP,
	"alex-dev": SCENARIO_DERP_FALLBACK,
	"priya-ml": SCENARIO_AGENT_CRASH,
};

export function getMockDiagnosticData(
	username: string,
): UserDiagnosticResponse {
	return SCENARIO_MAP[username] ?? SCENARIO_DEVICE_SLEEP;
}

export const MOCK_USERNAMES = Object.keys(SCENARIO_MAP);
