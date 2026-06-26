import { type FC, type ReactNode, useState } from "react";

import type {
	UpsertChatUsageLimitOverrideRequest,
	User,
} from "#/api/typesGenerated";
import {
	dollarsToMicros,
	isPositiveFiniteDollarAmount,
	microsToDollars,
} from "#/utils/currency";

interface EditingUserOverride {
	user_id: string;
	name: string;
	username: string;
	avatar_url: string;
}

type UserOverrideChildProps = {
	showUserForm: boolean;
	setShowUserForm: (show: boolean) => void;
	selectedUserOverride: User | null;
	setSelectedUserOverride: (user: User | null) => void;
	userOverrideAmount: string;
	setUserOverrideAmount: (amount: string) => void;
	editingUserOverride: EditingUserOverride | null;
	setEditingUserOverride: (override: EditingUserOverride | null) => void;
	handleShowUserFormChange: (show: boolean) => void;
	handleEditUserOverride: (
		override: EditingUserOverride & {
			spend_limit_micros: number | null;
		},
	) => void;
	handleAddOverride: () => void;
	existingUserIds: Set<string>;
	selectedUserAlreadyOverridden: boolean;
};

interface UserOverrideControllerProps {
	overrides: ReadonlyArray<{ user_id: string }>;
	onUpsertOverride: (args: {
		userID: string;
		req: UpsertChatUsageLimitOverrideRequest;
		onSuccess: () => void;
	}) => void;
	children: (props: UserOverrideChildProps) => ReactNode;
}

export const UserOverrideController: FC<UserOverrideControllerProps> = ({
	overrides,
	onUpsertOverride,
	children,
}) => {
	const [showUserForm, setShowUserForm] = useState(false);
	const [selectedUserOverride, setSelectedUserOverride] = useState<User | null>(
		null,
	);
	const [userOverrideAmount, setUserOverrideAmount] = useState("");
	const [editingUserOverride, setEditingUserOverride] =
		useState<EditingUserOverride | null>(null);

	// Derived values.
	const existingUserIds = new Set(overrides.map((o) => o.user_id));
	const selectedUserAlreadyOverridden = selectedUserOverride
		? existingUserIds.has(selectedUserOverride.id)
		: false;

	// Handlers.
	const handleShowUserFormChange = (show: boolean) => {
		setShowUserForm(show);
		if (!show) {
			setEditingUserOverride(null);
		}
	};

	const handleEditUserOverride = (
		override: EditingUserOverride & {
			spend_limit_micros: number | null;
		},
	) => {
		setEditingUserOverride({
			user_id: override.user_id,
			name: override.name,
			username: override.username,
			avatar_url: override.avatar_url,
		});
		setSelectedUserOverride(null);
		setUserOverrideAmount(
			override.spend_limit_micros !== null
				? microsToDollars(override.spend_limit_micros).toString()
				: "",
		);
		setShowUserForm(true);
	};

	const handleAddOverride = () => {
		const targetUserID =
			editingUserOverride?.user_id ?? selectedUserOverride?.id;

		if (!targetUserID || !isPositiveFiniteDollarAmount(userOverrideAmount)) {
			return;
		}
		onUpsertOverride({
			userID: targetUserID,
			req: { spend_limit_micros: dollarsToMicros(userOverrideAmount) },
			onSuccess: () => {
				setEditingUserOverride(null);
				setSelectedUserOverride(null);
				setUserOverrideAmount("");
				setShowUserForm(false);
			},
		});
	};

	return children({
		showUserForm,
		setShowUserForm,
		selectedUserOverride,
		setSelectedUserOverride,
		userOverrideAmount,
		setUserOverrideAmount,
		editingUserOverride,
		setEditingUserOverride,
		handleShowUserFormChange,
		handleEditUserOverride,
		handleAddOverride,
		existingUserIds,
		selectedUserAlreadyOverridden,
	});
};
