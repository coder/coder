import { type FC, useCallback, useEffect, useMemo } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { workspaces } from "#/api/queries/workspaces";
import type { Workspace } from "#/api/typesGenerated";
import { ACTIVE_BUILD_STATUSES } from "#/modules/workspaces/status";
import { MuxPageView } from "./MuxPageView";
import {
	filterMuxWorkspaces,
	getMuxCandidatesFromWorkspace,
	pickPreferredMuxApp,
} from "./muxApps";

const ACTIVE_MUX_REFRESH_INTERVAL = 5_000;
const IDLE_MUX_REFRESH_INTERVAL = 30_000;
const WORKSPACES_REQUEST = { q: "owner:me", limit: 0 } as const;

const MuxPage: FC = () => {
	const [searchParams, setSearchParams] = useSearchParams();
	const searchParamsKey = searchParams.toString();
	const selectedWorkspaceId = searchParams.get("workspace");
	const workspacesQueryOptions = workspaces(WORKSPACES_REQUEST);
	const workspacesQuery = useQuery({
		...workspacesQueryOptions,
		refetchInterval: ({ state }) => {
			if (state.error) {
				return false;
			}

			if (!state.data) {
				return ACTIVE_MUX_REFRESH_INTERVAL;
			}

			const muxCapableWorkspaces = getMuxCapableWorkspaces(
				state.data.workspaces,
			);
			const hasActiveBuild = muxCapableWorkspaces.some((workspace) =>
				ACTIVE_BUILD_STATUSES.includes(workspace.latest_build.status),
			);
			if (hasActiveBuild) {
				return ACTIVE_MUX_REFRESH_INTERVAL;
			}

			const muxWorkspaces = filterMuxWorkspaces(state.data.workspaces);
			const selectedWorkspace = getSelectedWorkspace(
				muxWorkspaces,
				selectedWorkspaceId,
			);
			const selectedMuxCandidate = selectedWorkspace
				? pickPreferredMuxApp(getMuxCandidatesFromWorkspace(selectedWorkspace))
				: undefined;

			return selectedMuxCandidate?.app.health === "initializing"
				? ACTIVE_MUX_REFRESH_INTERVAL
				: IDLE_MUX_REFRESH_INTERVAL;
		},
		refetchOnWindowFocus: "always",
	});

	const muxWorkspaces = useMemo(
		() => filterMuxWorkspaces(workspacesQuery.data?.workspaces ?? []),
		[workspacesQuery.data?.workspaces],
	);

	useEffect(() => {
		if (!workspacesQuery.data) {
			return;
		}

		const selectedParamIsValid =
			selectedWorkspaceId !== null &&
			muxWorkspaces.some((workspace) => workspace.id === selectedWorkspaceId);
		const onlyMuxWorkspace =
			muxWorkspaces.length === 1 ? muxWorkspaces[0] : undefined;

		if (onlyMuxWorkspace && selectedWorkspaceId !== onlyMuxWorkspace.id) {
			const nextParams = new URLSearchParams(searchParamsKey);
			nextParams.set("workspace", onlyMuxWorkspace.id);
			setSearchParams(nextParams, { replace: true });
			return;
		}

		if (selectedWorkspaceId !== null && !selectedParamIsValid) {
			const nextParams = new URLSearchParams(searchParamsKey);
			nextParams.delete("workspace");
			setSearchParams(nextParams, { replace: true });
		}
	}, [
		muxWorkspaces,
		searchParamsKey,
		selectedWorkspaceId,
		setSearchParams,
		workspacesQuery.data,
	]);

	const selectedWorkspace = useMemo(
		() => getSelectedWorkspace(muxWorkspaces, selectedWorkspaceId),
		[muxWorkspaces, selectedWorkspaceId],
	);
	const selectedMuxCandidate = useMemo(() => {
		if (!selectedWorkspace) {
			return undefined;
		}

		return pickPreferredMuxApp(
			getMuxCandidatesFromWorkspace(selectedWorkspace),
		);
	}, [selectedWorkspace]);

	const handleSelectWorkspace = useCallback(
		(workspaceId: string | undefined) => {
			const nextParams = new URLSearchParams(searchParamsKey);
			if (workspaceId) {
				nextParams.set("workspace", workspaceId);
			} else {
				nextParams.delete("workspace");
			}

			setSearchParams(nextParams);
		},
		[searchParamsKey, setSearchParams],
	);

	return (
		<MuxPageView
			isLoading={workspacesQuery.isLoading}
			error={workspacesQuery.error}
			ownedWorkspaceCount={workspacesQuery.data?.count}
			muxWorkspaces={muxWorkspaces}
			selectedWorkspace={selectedWorkspace}
			selectedMuxCandidate={selectedMuxCandidate}
			onSelectWorkspace={handleSelectWorkspace}
		/>
	);
};

const getSelectedWorkspace = (
	muxWorkspaces: readonly Workspace[],
	selectedWorkspaceId: string | null,
): Workspace | undefined => {
	if (!selectedWorkspaceId) {
		return undefined;
	}

	return muxWorkspaces.find(
		(workspace) => workspace.id === selectedWorkspaceId,
	);
};

const getMuxCapableWorkspaces = (
	workspaces: readonly Workspace[],
): Workspace[] => {
	return workspaces.filter((workspace) => {
		return (
			workspace.dormant_at === null &&
			!workspace.is_prebuild &&
			getMuxCandidatesFromWorkspace(workspace).length > 0
		);
	});
};

export default MuxPage;
