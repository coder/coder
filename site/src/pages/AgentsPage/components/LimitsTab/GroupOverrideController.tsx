import { type FC, type ReactNode, useState } from "react";

import type {
	Group,
	UpsertChatUsageLimitGroupOverrideRequest,
} from "#/api/typesGenerated";
import {
	dollarsToMicros,
	isPositiveFiniteDollarAmount,
	microsToDollars,
} from "#/utils/currency";

interface EditingGroupOverride {
	group_id: string;
	group_display_name: string;
	group_name: string;
	group_avatar_url: string;
	member_count: number;
}

type GroupOverrideChildProps = {
	showGroupForm: boolean;
	setShowGroupForm: (show: boolean) => void;
	selectedGroup: Group | null;
	setSelectedGroup: (group: Group | null) => void;
	groupAmount: string;
	setGroupAmount: (amount: string) => void;
	editingGroupOverride: EditingGroupOverride | null;
	setEditingGroupOverride: (override: EditingGroupOverride | null) => void;
	handleShowGroupFormChange: (show: boolean) => void;
	handleEditGroupOverride: (
		override: EditingGroupOverride & {
			spend_limit_micros: number | null;
		},
	) => void;
	handleAddGroupOverride: () => void;
	existingGroupIds: Set<string>;
	availableGroups: Group[];
	groupAutocompleteNoOptionsText: string;
	groupOrganizationNames: Record<string, string>;
};

interface GroupOverrideControllerProps {
	groupOverrides: ReadonlyArray<{ group_id: string }>;
	groups: ReadonlyArray<Group>;
	isLoadingGroups: boolean;
	onUpsertGroupOverride: (args: {
		groupID: string;
		req: UpsertChatUsageLimitGroupOverrideRequest;
		onSuccess: () => void;
	}) => void;
	children: (props: GroupOverrideChildProps) => ReactNode;
}

export const GroupOverrideController: FC<GroupOverrideControllerProps> = ({
	groupOverrides,
	groups,
	isLoadingGroups,
	onUpsertGroupOverride,
	children,
}) => {
	const [showGroupForm, setShowGroupForm] = useState(false);
	const [selectedGroup, setSelectedGroup] = useState<Group | null>(null);
	const [groupAmount, setGroupAmount] = useState("");
	const [editingGroupOverride, setEditingGroupOverride] =
		useState<EditingGroupOverride | null>(null);

	// Derived values.
	const existingGroupIds = new Set(groupOverrides.map((g) => g.group_id));
	const availableGroups = groups.filter((g) => !existingGroupIds.has(g.id));
	const groupOrganizationNames: Record<string, string> = {};
	for (const g of groups) {
		groupOrganizationNames[g.id] = g.organization_name;
	}
	const groupAutocompleteNoOptionsText = isLoadingGroups
		? "Loading groups..."
		: groups.length === 0
			? "No groups configured"
			: availableGroups.length === 0
				? "All groups already have overrides"
				: "No groups available";

	// Handlers.
	const handleShowGroupFormChange = (show: boolean) => {
		setShowGroupForm(show);
		if (!show) {
			setEditingGroupOverride(null);
		}
	};

	const handleEditGroupOverride = (
		override: EditingGroupOverride & {
			spend_limit_micros: number | null;
		},
	) => {
		setEditingGroupOverride({
			group_id: override.group_id,
			group_display_name: override.group_display_name,
			group_name: override.group_name,
			group_avatar_url: override.group_avatar_url,
			member_count: override.member_count,
		});
		setSelectedGroup(null);
		setGroupAmount(
			override.spend_limit_micros !== null
				? microsToDollars(override.spend_limit_micros).toString()
				: "",
		);
		setShowGroupForm(true);
	};

	const handleAddGroupOverride = () => {
		const targetGroupID = editingGroupOverride?.group_id ?? selectedGroup?.id;

		if (!targetGroupID || !isPositiveFiniteDollarAmount(groupAmount)) {
			return;
		}
		onUpsertGroupOverride({
			groupID: targetGroupID,
			req: { spend_limit_micros: dollarsToMicros(groupAmount) },
			onSuccess: () => {
				setEditingGroupOverride(null);
				setSelectedGroup(null);
				setGroupAmount("");
				setShowGroupForm(false);
			},
		});
	};

	return children({
		showGroupForm,
		setShowGroupForm,
		selectedGroup,
		setSelectedGroup,
		groupAmount,
		setGroupAmount,
		editingGroupOverride,
		setEditingGroupOverride,
		handleShowGroupFormChange,
		handleEditGroupOverride,
		handleAddGroupOverride,
		existingGroupIds,
		availableGroups,
		groupAutocompleteNoOptionsText,
		groupOrganizationNames,
	});
};
