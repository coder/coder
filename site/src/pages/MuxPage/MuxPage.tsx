import { type FC, useCallback, useEffect, useMemo, useState } from "react";
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
	const launchedWorkspaceId = searchParams.get("workspace");
	const [launcherWorkspaceId, setLauncherWorkspaceId] = useState<
		string | undefined
	>();
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
			const launchedWorkspace = getSelectedWorkspace(
				muxWorkspaces,
				launchedWorkspaceId,
			);
			const launchedMuxCandidate = launchedWorkspace
				? pickPreferredMuxApp(getMuxCandidatesFromWorkspace(launchedWorkspace))
				: undefined;

			return launchedMuxCandidate?.app.health === "initializing"
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

		const launchedParamIsValid =
			launchedWorkspaceId !== null &&
			muxWorkspaces.some((workspace) => workspace.id === launchedWorkspaceId);

		if (launchedWorkspaceId !== null && !launchedParamIsValid) {
			const nextParams = new URLSearchParams(searchParamsKey);
			nextParams.delete("workspace");
			setSearchParams(nextParams, { replace: true });
		}
	}, [
		muxWorkspaces,
		searchParamsKey,
		launchedWorkspaceId,
		setSearchParams,
		workspacesQuery.data,
	]);

	useEffect(() => {
		if (!workspacesQuery.data || !launcherWorkspaceId) {
			return;
		}

		const launcherSelectionIsValid = muxWorkspaces.some(
			(workspace) => workspace.id === launcherWorkspaceId,
		);
		if (!launcherSelectionIsValid) {
			setLauncherWorkspaceId(undefined);
		}
	}, [launcherWorkspaceId, muxWorkspaces, workspacesQuery.data]);

	const launchedWorkspace = useMemo(
		() => getSelectedWorkspace(muxWorkspaces, launchedWorkspaceId),
		[muxWorkspaces, launchedWorkspaceId],
	);
	const launchedMuxCandidate = useMemo(() => {
		if (!launchedWorkspace) {
			return undefined;
		}

		return pickPreferredMuxApp(
			getMuxCandidatesFromWorkspace(launchedWorkspace),
		);
	}, [launchedWorkspace]);
	const isLaunched =
		launchedWorkspaceId !== null &&
		launchedWorkspace !== undefined &&
		launchedWorkspace.latest_build.status !== "stopped" &&
		launchedMuxCandidate !== undefined;

	const launcherSelectedWorkspaceId =
		launcherWorkspaceId ??
		(isLaunched ? undefined : (launchedWorkspaceId ?? undefined));
	const launcherWorkspace = useMemo(
		() => getSelectedWorkspace(muxWorkspaces, launcherSelectedWorkspaceId),
		[muxWorkspaces, launcherSelectedWorkspaceId],
	);
	const launcherMuxCandidate = useMemo(() => {
		if (!launcherWorkspace) {
			return undefined;
		}

		return pickPreferredMuxApp(
			getMuxCandidatesFromWorkspace(launcherWorkspace),
		);
	}, [launcherWorkspace]);

	const handleSelectWorkspace = useCallback(
		(workspaceId: string | undefined) => {
			setLauncherWorkspaceId(workspaceId);

			if (launchedWorkspaceId === null) {
				return;
			}

			const nextParams = new URLSearchParams(searchParamsKey);
			nextParams.delete("workspace");
			setSearchParams(nextParams);
		},
		[launchedWorkspaceId, searchParamsKey, setSearchParams],
	);

	const handleLaunchWorkspace = useCallback(() => {
		if (!launcherSelectedWorkspaceId) {
			return;
		}

		const nextParams = new URLSearchParams(searchParamsKey);
		nextParams.set("workspace", launcherSelectedWorkspaceId);
		setSearchParams(nextParams);
	}, [launcherSelectedWorkspaceId, searchParamsKey, setSearchParams]);

	const handleChangeWorkspace = useCallback(() => {
		setLauncherWorkspaceId(undefined);

		const nextParams = new URLSearchParams(searchParamsKey);
		nextParams.delete("workspace");
		setSearchParams(nextParams);
	}, [searchParamsKey, setSearchParams]);

	return (
		<MuxPageView
			isLoading={workspacesQuery.isLoading}
			error={workspacesQuery.error}
			ownedWorkspaceCount={workspacesQuery.data?.count}
			muxWorkspaces={muxWorkspaces}
			selectedWorkspace={isLaunched ? launchedWorkspace : launcherWorkspace}
			selectedMuxCandidate={
				isLaunched ? launchedMuxCandidate : launcherMuxCandidate
			}
			isLaunched={isLaunched}
			onSelectWorkspace={handleSelectWorkspace}
			onLaunchWorkspace={handleLaunchWorkspace}
			onChangeWorkspace={handleChangeWorkspace}
		/>
	);
};

const getSelectedWorkspace = (
	muxWorkspaces: readonly Workspace[],
	selectedWorkspaceId: string | null | undefined,
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
