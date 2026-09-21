import { DownloadIcon } from "lucide-react";
import type { ComponentProps, FC } from "react";
import type {
	Organization,
	OrganizationAISpendDetails,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import {
	PaginationContainer,
	type PaginationResult,
} from "#/components/PaginationWidget/PaginationContainer";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import { PremiumPaywallAIGovernance } from "#/modules/paywall/PremiumPaywallAIGovernance";
import { AIBridgeSetupAlert } from "#/pages/AIBridgePage/AIBridgeSetupAlert";
import { SpendDetailsFilters } from "./components/SpendDetailsFilters";
import { SpendDetailsTable } from "./components/SpendDetailsTable";

export type SpendDetailsQuery = PaginationResult<OrganizationAISpendDetails> & {
	isLoading: boolean;
	isFetching: boolean;
	error: unknown;
	refetch: () => unknown;
};

interface SpendDetailsPageViewProps {
	// Entitlement
	isEntitled: boolean;
	isEnabled: boolean;
	// Organization selection
	organization: Organization | undefined;
	isOrganizationsLoading: boolean;
	requestedOrganizationDenied: boolean;
	organizationsError: unknown;
	// Filters (organization is supplied separately once a report can render)
	filters: Omit<ComponentProps<typeof SpendDetailsFilters>, "organization">;
	// Export
	onExport: () => void;
	isExporting: boolean;
	canExport: boolean;
	exportError: unknown;
	// Data
	query: SpendDetailsQuery;
}

export const SpendDetailsPageView: FC<SpendDetailsPageViewProps> = ({
	isEntitled,
	isEnabled,
	organization,
	isOrganizationsLoading,
	requestedOrganizationDenied,
	organizationsError,
	filters,
	onExport,
	isExporting,
	canExport,
	exportError,
	query,
}) => {
	if (!isEntitled) {
		return (
			<PremiumPaywallAIGovernance variant="governance" source="ai_governance" />
		);
	}

	if (!isEnabled) {
		return <AIBridgeSetupAlert />;
	}

	return (
		<div className="flex max-w-[1400px] flex-col gap-4">
			<SettingsHeader
				actions={
					organization ? (
						<Button
							variant="outline"
							size="lg"
							onClick={onExport}
							disabled={!canExport || isExporting}
						>
							<DownloadIcon />
							Export CSV
						</Button>
					) : undefined
				}
			>
				<SettingsHeaderTitle>AI spend details</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Review AI Gateway spend and token usage by user, effective group,
					provider, and model.
				</SettingsHeaderDescription>
			</SettingsHeader>
			<SpendDetailsBody
				organization={organization}
				isOrganizationsLoading={isOrganizationsLoading}
				requestedOrganizationDenied={requestedOrganizationDenied}
				organizationsError={organizationsError}
				exportError={exportError}
				filters={filters}
				query={query}
			/>
		</div>
	);
};

interface SpendDetailsBodyProps {
	organization: Organization | undefined;
	isOrganizationsLoading: boolean;
	requestedOrganizationDenied: boolean;
	organizationsError: unknown;
	exportError: unknown;
	filters: Omit<ComponentProps<typeof SpendDetailsFilters>, "organization">;
	query: SpendDetailsQuery;
}

const SpendDetailsBody: FC<SpendDetailsBodyProps> = ({
	organization,
	isOrganizationsLoading,
	requestedOrganizationDenied,
	organizationsError,
	exportError,
	filters,
	query,
}) => {
	if (isOrganizationsLoading) {
		return <Loader />;
	}

	if (requestedOrganizationDenied) {
		return (
			<ErrorAlert
				error={
					new Error("You do not have access to the requested organization.")
				}
				showDebugDetail={false}
			/>
		);
	}

	if (organization === undefined) {
		return organizationsError ? (
			<ErrorAlert error={organizationsError} />
		) : (
			<p className="py-12 text-center text-content-secondary">
				You do not have access to any organization&apos;s AI spend.
			</p>
		);
	}

	return (
		<>
			{organizationsError != null && <ErrorAlert error={organizationsError} />}
			{exportError != null && <ErrorAlert error={exportError} />}
			<SpendDetailsFilters {...filters} organization={organization} />
			<SpendDetailsResults query={query} />
		</>
	);
};

const SpendDetailsResults: FC<{ query: SpendDetailsQuery }> = ({ query }) => {
	const retryButton = (
		<Button variant="outline" size="sm" onClick={() => void query.refetch()}>
			Retry
		</Button>
	);

	if (query.error && !query.data) {
		return (
			<div className="flex min-h-[240px] flex-col items-center justify-center gap-4">
				<ErrorAlert error={query.error} />
				{retryButton}
			</div>
		);
	}

	return (
		<>
			{query.error != null && (
				<ErrorAlert error={query.error} actions={retryButton} />
			)}
			<PaginationContainer query={query} paginationUnitLabel="rows">
				<div className="relative">
					{query.isFetching && !query.isLoading && (
						<div
							role="status"
							aria-label="Refreshing spend details"
							className="absolute inset-0 z-10 flex items-center justify-center bg-surface-primary/50"
						>
							<Spinner size="lg" loading className="text-content-secondary" />
						</div>
					)}
					<SpendDetailsTable
						rows={query.data?.rows}
						isLoading={query.isLoading}
					/>
				</div>
			</PaginationContainer>
		</>
	);
};
