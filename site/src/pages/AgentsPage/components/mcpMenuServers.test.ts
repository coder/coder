import { describe, expect, it } from "vitest";
import type { MCPServerConfig } from "#/api/typesGenerated";
import { MockMCPServerConfig } from "#/testHelpers/chatEntities";
import {
	countMCPServersNeedingSignIn,
	getMCPMenuServers,
} from "./mcpMenuServers";

describe("getMCPMenuServers", () => {
	const mockAPIKeyServer: MCPServerConfig = {
		...MockMCPServerConfig,
		id: "linear",
		auth_type: "api_key",
	};
	const mockConnectedServer: MCPServerConfig = {
		...MockMCPServerConfig,
		id: "github",
		auth_type: "oauth2",
		auth_connected: true,
	};
	const mockSignedOutSelectedServer: MCPServerConfig = {
		...MockMCPServerConfig,
		id: "notion",
		auth_type: "oauth2",
		auth_connected: false,
	};
	const mockSignedOutForcedServer: MCPServerConfig = {
		...MockMCPServerConfig,
		id: "slack",
		auth_type: "oauth2",
		auth_connected: false,
		availability: "force_on",
	};
	const mockSignedOutUnselectedServer: MCPServerConfig = {
		...MockMCPServerConfig,
		id: "wiz",
		auth_type: "oauth2",
		auth_connected: false,
	};
	const mockUnsharedForcedServer: MCPServerConfig = {
		...MockMCPServerConfig,
		id: "jira",
		availability: "force_on",
		acl_granted: false,
	};
	const mockServers = [
		mockAPIKeyServer,
		mockConnectedServer,
		mockSignedOutSelectedServer,
		mockSignedOutForcedServer,
		mockSignedOutUnselectedServer,
	];

	it("lists usable servers and selected or forced servers that are signed out", () => {
		expect(
			getMCPMenuServers(mockServers, [
				mockConnectedServer.id,
				mockSignedOutSelectedServer.id,
			]),
		).toEqual([
			{ server: mockAPIKeyServer, isSelected: false, isDisconnected: false },
			{ server: mockConnectedServer, isSelected: true, isDisconnected: false },
			{
				server: mockSignedOutSelectedServer,
				isSelected: true,
				isDisconnected: true,
			},
			{
				server: mockSignedOutForcedServer,
				isSelected: true,
				isDisconnected: true,
			},
		]);
	});

	it("leaves force_on servers whose ACL excludes the caller unselected", () => {
		expect(getMCPMenuServers([mockUnsharedForcedServer], [])).toEqual([
			{
				server: mockUnsharedForcedServer,
				isSelected: false,
				isDisconnected: false,
			},
		]);
	});

	it("counts every server that needs sign-in", () => {
		expect(countMCPServersNeedingSignIn(mockServers)).toBe(3);
	});
});
