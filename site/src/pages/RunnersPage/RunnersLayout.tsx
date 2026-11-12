import { createContext, useContext, useState } from "react";
import { Outlet } from "react-router";
import { Alert } from "#/components/Alert/Alert";
import { Margins } from "#/components/Margins/Margins";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { canViewDeploymentSettings } from "#/modules/permissions";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { type DemoRunner, loadRunners, runnersStorageKey } from "./demoData";

type RunnersContextValue = {
	runners: DemoRunner[];
	saveRunner: (runner: DemoRunner) => void;
	deleteRunner: (id: string) => void;
	now: number;
};

const RunnersContext = createContext<RunnersContextValue | null>(null);

/** Local configuration shared by all runner demo routes. */
export function useRunners() {
	const context = useContext(RunnersContext);
	if (!context) throw new Error("useRunners requires RunnersLayout");
	return context;
}

const RunnersLayout: React.FC = () => {
	const { permissions } = useAuthenticated();
	return (
		<RequirePermission
			isFeatureVisible={canViewDeploymentSettings(permissions)}
		>
			<RunnerConfiguration />
		</RequirePermission>
	);
};

const RunnerConfiguration: React.FC = () => {
	const [state, setState] = useState(loadRunners);
	const [now] = useState(() => Date.now());
	const updateRunners = (runners: DemoRunner[]) => {
		let storageUnavailable = false;
		try {
			localStorage.setItem(runnersStorageKey, JSON.stringify(runners));
		} catch {
			storageUnavailable = true;
		}
		setState({ runners, storageUnavailable });
	};
	return (
		<RunnersContext.Provider
			value={{
				runners: state.runners,
				now,
				saveRunner: (runner) =>
					updateRunners(
						state.runners.some((item) => item.id === runner.id)
							? state.runners.map((item) =>
									item.id === runner.id ? runner : item,
								)
							: [...state.runners, runner],
					),
				deleteRunner: (id) =>
					updateRunners(state.runners.filter((runner) => runner.id !== id)),
			}}
		>
			{state.storageUnavailable && (
				<Margins className="pt-6">
					<Alert severity="warning">
						Browser storage is unavailable. Changes will last until you refresh
						this page.
					</Alert>
				</Margins>
			)}
			<Outlet />
		</RunnersContext.Provider>
	);
};

export default RunnersLayout;
