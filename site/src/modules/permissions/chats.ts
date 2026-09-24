export const chatPermissionChecks = (organizationId: string) =>
	({
		createChatInOrganization: {
			object: {
				resource_type: "chat",
				organization_id: organizationId,
				owner_id: "me",
			},
			action: "create",
		},
	}) as const;
