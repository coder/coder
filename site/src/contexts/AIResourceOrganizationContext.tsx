import {
	createContext,
	type FC,
	type PropsWithChildren,
	useContext,
	useEffect,
	useMemo,
	useRef,
} from "react";
import { useQuery } from "react-query";
import { Outlet, useOutletContext, useSearchParams } from "react-router";
import { aiResourceOrganizationPermissions } from "#/api/queries/organizations";
import type { Organization } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Loader } from "#/components/Loader/Loader";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import type { AIResourceOrganizationPermissions } from "#/modules/permissions/organizations";

const organizationSearchParam = "organization";

export const resolveAIResourceOrganization = (
	organizations: readonly Organization[],
	organizationName: string | null,
): Organization | undefined => {
	const selected = organizationName
		? organizations.find(
				(organization) => organization.name === organizationName,
			)
		: undefined;
	return (
		selected ??
		organizations.find((organization) => organization.is_default) ??
		organizations[0]
	);
};

type OrganizationChangeGuard = (
	nextOrganization: Organization,
) => boolean | Promise<boolean>;

type AIResourceOrganizationContextValue = Readonly<{
	organizations: readonly Organization[];
	organization: Organization;
	permissions: AIResourceOrganizationPermissions;
	permissionsByOrganizationId: Record<
		string,
		AIResourceOrganizationPermissions
	>;
	selectOrganization: (organization: Organization) => Promise<void>;
	registerOrganizationChangeGuard: (
		guard: OrganizationChangeGuard,
	) => () => void;
}>;

const AIResourceOrganizationContext = createContext<
	AIResourceOrganizationContextValue | undefined
>(undefined);

type AIResourceOrganizationProviderProps = PropsWithChildren<{
	isOrganizationPermitted: (
		permissions: AIResourceOrganizationPermissions,
	) => boolean;
}>;

export const AIResourceOrganizationProvider: FC<
	AIResourceOrganizationProviderProps
> = ({ children, isOrganizationPermitted }) => {
	const { organizations } = useDashboard();
	const [searchParams, setSearchParams] = useSearchParams();
	const organizationChangeGuardRef = useRef<
		OrganizationChangeGuard | undefined
	>(undefined);
	const organizationIds = useMemo(
		() => organizations.map((organization) => organization.id),
		[organizations],
	);
	const permissionsQuery = useQuery(
		aiResourceOrganizationPermissions(organizationIds),
	);
	const permittedOrganizations = organizations.filter((organization) => {
		const permissions = permissionsQuery.data?.[organization.id];
		return permissions ? isOrganizationPermitted(permissions) : false;
	});
	const organization = resolveAIResourceOrganization(
		permittedOrganizations,
		searchParams.get(organizationSearchParam),
	);

	useEffect(() => {
		if (!organization) {
			return;
		}
		if (searchParams.get(organizationSearchParam) === organization.name) {
			return;
		}
		setSearchParams(
			(current) => {
				const next = new URLSearchParams(current);
				next.set(organizationSearchParam, organization.name);
				return next;
			},
			{ replace: true },
		);
	}, [organization, searchParams, setSearchParams]);

	if (organizations.length === 0) {
		return <EmptyState message="No permitted organizations found" />;
	}
	if (permissionsQuery.isLoading) {
		return <Loader />;
	}
	if (permissionsQuery.error) {
		return <ErrorAlert error={permissionsQuery.error} />;
	}
	if (!organization) {
		return <EmptyState message="No permitted organizations found" />;
	}

	const selectOrganization = async (nextOrganization: Organization) => {
		if (
			organizationChangeGuardRef.current &&
			!(await organizationChangeGuardRef.current(nextOrganization))
		) {
			return;
		}
		setSearchParams((current) => {
			const next = new URLSearchParams(current);
			next.set(organizationSearchParam, nextOrganization.name);
			return next;
		});
	};
	const registerOrganizationChangeGuard = (guard: OrganizationChangeGuard) => {
		organizationChangeGuardRef.current = guard;
		return () => {
			if (organizationChangeGuardRef.current === guard) {
				organizationChangeGuardRef.current = undefined;
			}
		};
	};

	return (
		<AIResourceOrganizationContext.Provider
			value={{
				organizations: permittedOrganizations,
				organization,
				permissions: permissionsQuery.data?.[
					organization.id
				] as AIResourceOrganizationPermissions,
				permissionsByOrganizationId: permissionsQuery.data ?? {},
				selectOrganization,
				registerOrganizationChangeGuard,
			}}
		>
			{children}
		</AIResourceOrganizationContext.Provider>
	);
};

type AIResourceOrganizationOutletProps = {
	isOrganizationPermitted: (
		permissions: AIResourceOrganizationPermissions,
	) => boolean;
};

export const AIResourceOrganizationOutlet: FC<
	AIResourceOrganizationOutletProps
> = ({ isOrganizationPermitted }) => {
	const outletContext = useOutletContext();
	return (
		<AIResourceOrganizationProvider
			isOrganizationPermitted={isOrganizationPermitted}
		>
			<Outlet context={outletContext} />
		</AIResourceOrganizationProvider>
	);
};

export const useAIResourceOrganization = () => {
	const context = useContext(AIResourceOrganizationContext);
	if (!context) {
		throw new Error(
			"useAIResourceOrganization must be used inside AIResourceOrganizationProvider",
		);
	}
	return context;
};
