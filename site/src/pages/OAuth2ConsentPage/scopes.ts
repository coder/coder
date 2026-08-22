/**
 * Human descriptions for the external scope catalog.
 *
 * The catalog itself lives in `coderd/rbac/scopes_catalog.go` — `externalLowLevel`
 * for `resource:action` names and `externalComposite` for the curated `coder:*`
 * names. The backend hands the consent page raw scope strings (see the `Scopes`
 * field added to `RenderOAuthAllowData` in coder/coder#28045); this module turns
 * them into something a user can make a decision about.
 *
 * Keep this in sync with the catalog. An unknown scope is shown verbatim rather
 * than hidden — the user should never approve a permission the page couldn't
 * describe without being told that's what happened.
 */

export type ScopeRisk = "read" | "write" | "destructive" | "unrestricted";

export type DescribedScope = {
	/** Raw scope string, e.g. `workspace:read`. */
	name: string;
	/** Sentence-case description of what the scope allows. */
	description: string;
	risk: ScopeRisk;
	/** False when the scope isn't in the catalog this page knows about. */
	known: boolean;
};

export type ScopeGroup = {
	/** Resource the scopes apply to, e.g. "Workspaces". */
	title: string;
	scopes: DescribedScope[];
};

type CatalogEntry = {
	group: string;
	description: string;
	risk: ScopeRisk;
};

const catalog: Record<string, CatalogEntry> = {
	// Unrestricted
	all: {
		group: "Everything",
		description: "Full access to your account",
		risk: "unrestricted",
	},
	application_connect: {
		group: "Workspaces",
		description: "Open apps running in your workspaces",
		risk: "read",
	},

	// Workspaces
	"workspace:read": {
		group: "Workspaces",
		description: "View your workspaces and their status",
		risk: "read",
	},
	"workspace:create": {
		group: "Workspaces",
		description: "Create new workspaces",
		risk: "write",
	},
	"workspace:update": {
		group: "Workspaces",
		description: "Change your workspace settings",
		risk: "write",
	},
	"workspace:delete": {
		group: "Workspaces",
		description: "Delete your workspaces",
		risk: "destructive",
	},
	"workspace:ssh": {
		group: "Workspaces",
		description: "Connect to your workspaces over SSH",
		risk: "write",
	},
	"workspace:start": {
		group: "Workspaces",
		description: "Start your workspaces",
		risk: "write",
	},
	"workspace:stop": {
		group: "Workspaces",
		description: "Stop your workspaces",
		risk: "write",
	},
	"workspace:application_connect": {
		group: "Workspaces",
		description: "Open apps running in your workspaces",
		risk: "read",
	},
	"workspace:*": {
		group: "Workspaces",
		description: "Full access to your workspaces, including deleting them",
		risk: "destructive",
	},

	// Templates
	"template:read": {
		group: "Templates",
		description: "View templates you have access to",
		risk: "read",
	},
	"template:use": {
		group: "Templates",
		description: "Build workspaces from templates",
		risk: "write",
	},
	"template:create": {
		group: "Templates",
		description: "Create templates",
		risk: "write",
	},
	"template:update": {
		group: "Templates",
		description: "Change templates",
		risk: "write",
	},
	"template:delete": {
		group: "Templates",
		description: "Delete templates",
		risk: "destructive",
	},
	"template:*": {
		group: "Templates",
		description: "Full access to templates, including deleting them",
		risk: "destructive",
	},

	// API keys
	"api_key:read": {
		group: "API keys",
		description: "View your API keys",
		risk: "read",
	},
	"api_key:create": {
		group: "API keys",
		description: "Create API keys on your behalf",
		risk: "write",
	},
	"api_key:update": {
		group: "API keys",
		description: "Change your API keys",
		risk: "write",
	},
	"api_key:delete": {
		group: "API keys",
		description: "Delete your API keys",
		risk: "destructive",
	},
	"api_key:*": {
		group: "API keys",
		description: "Full access to your API keys",
		risk: "destructive",
	},

	// Files
	"file:read": {
		group: "Files",
		description: "Read files you have uploaded",
		risk: "read",
	},
	"file:create": {
		group: "Files",
		description: "Upload files",
		risk: "write",
	},
	"file:*": {
		group: "Files",
		description: "Full access to your files",
		risk: "write",
	},

	// Profile
	"user:read": {
		group: "Profile",
		description: "View your name and profile",
		risk: "read",
	},
	"user:read_personal": {
		group: "Profile",
		description: "View your email address and personal details",
		risk: "read",
	},
	"user:update_personal": {
		group: "Profile",
		description: "Change your personal details",
		risk: "write",
	},
	"user:*": {
		group: "Profile",
		description: "Full access to your profile",
		risk: "write",
	},

	// Secrets
	"user_secret:read": {
		group: "Secrets",
		description: "Read secrets stored in your account",
		risk: "read",
	},
	"user_secret:create": {
		group: "Secrets",
		description: "Add secrets to your account",
		risk: "write",
	},
	"user_secret:update": {
		group: "Secrets",
		description: "Change secrets stored in your account",
		risk: "write",
	},
	"user_secret:delete": {
		group: "Secrets",
		description: "Delete secrets stored in your account",
		risk: "destructive",
	},
	"user_secret:*": {
		group: "Secrets",
		description: "Full access to secrets stored in your account",
		risk: "destructive",
	},

	// Skills
	"user_skill:read": {
		group: "Skills",
		description: "View your skills",
		risk: "read",
	},
	"user_skill:create": {
		group: "Skills",
		description: "Add skills to your account",
		risk: "write",
	},
	"user_skill:update": {
		group: "Skills",
		description: "Change your skills",
		risk: "write",
	},
	"user_skill:delete": {
		group: "Skills",
		description: "Delete your skills",
		risk: "destructive",
	},
	"user_skill:*": {
		group: "Skills",
		description: "Full access to your skills",
		risk: "destructive",
	},

	// Tasks
	"task:read": {
		group: "Tasks",
		description: "View your tasks",
		risk: "read",
	},
	"task:create": {
		group: "Tasks",
		description: "Create tasks",
		risk: "write",
	},
	"task:update": {
		group: "Tasks",
		description: "Change your tasks",
		risk: "write",
	},
	"task:delete": {
		group: "Tasks",
		description: "Delete your tasks",
		risk: "destructive",
	},
	"task:*": {
		group: "Tasks",
		description: "Full access to your tasks",
		risk: "destructive",
	},

	// Organizations
	"organization:read": {
		group: "Organizations",
		description: "View organizations you belong to",
		risk: "read",
	},
	"organization:update": {
		group: "Organizations",
		description: "Change organization settings",
		risk: "write",
	},
	"organization:delete": {
		group: "Organizations",
		description: "Delete organizations",
		risk: "destructive",
	},
	"organization:*": {
		group: "Organizations",
		description: "Full access to organizations you belong to",
		risk: "destructive",
	},

	// Composite coder:* scopes
	"coder:workspaces.create": {
		group: "Workspaces",
		description: "Create and start workspaces",
		risk: "write",
	},
	"coder:workspaces.operate": {
		group: "Workspaces",
		description: "Start, stop and connect to your workspaces",
		risk: "write",
	},
	"coder:workspaces.delete": {
		group: "Workspaces",
		description: "Delete your workspaces",
		risk: "destructive",
	},
	"coder:workspaces.access": {
		group: "Workspaces",
		description: "Connect to your workspaces and the apps running in them",
		risk: "write",
	},
	"coder:templates.build": {
		group: "Templates",
		description: "Build workspaces from templates",
		risk: "write",
	},
	"coder:templates.author": {
		group: "Templates",
		description: "Create and change templates",
		risk: "write",
	},
	"coder:apikeys.manage_self": {
		group: "API keys",
		description: "Manage your own API keys",
		risk: "write",
	},
};

/** Groups are listed in this order; anything unlisted follows alphabetically. */
const groupOrder = [
	"Everything",
	"Workspaces",
	"Templates",
	"Tasks",
	"Files",
	"Secrets",
	"Skills",
	"Profile",
	"API keys",
	"Organizations",
];

const riskOrder: Record<ScopeRisk, number> = {
	unrestricted: 0,
	destructive: 1,
	write: 2,
	read: 3,
};

export const describeScope = (name: string): DescribedScope => {
	const entry = catalog[name];
	if (!entry) {
		return {
			name,
			// Shown rather than hidden: approving an undescribed permission
			// silently is worse than admitting the page doesn't know it.
			description: "This deployment didn't describe this permission",
			risk: "write",
			known: false,
		};
	}
	return {
		name,
		description: entry.description,
		risk: entry.risk,
		known: true,
	};
};

/**
 * Groups scopes by resource, most consequential first within each group, and
 * orders the groups so the ones users care most about come first.
 */
export const groupScopes = (names: readonly string[]): ScopeGroup[] => {
	const groups = new Map<string, DescribedScope[]>();

	for (const name of names) {
		const title = catalog[name]?.group ?? "Other";
		const scopes = groups.get(title) ?? [];
		scopes.push(describeScope(name));
		groups.set(title, scopes);
	}

	for (const scopes of groups.values()) {
		scopes.sort((a, b) => riskOrder[a.risk] - riskOrder[b.risk]);
	}

	return [...groups.entries()]
		.map(([title, scopes]) => ({ title, scopes }))
		.sort((a, b) => {
			const ai = groupOrder.indexOf(a.title);
			const bi = groupOrder.indexOf(b.title);
			if (ai === -1 && bi === -1) {
				return a.title.localeCompare(b.title);
			}
			if (ai === -1) {
				return 1;
			}
			if (bi === -1) {
				return -1;
			}
			return ai - bi;
		});
};

/**
 * True when the request amounts to unrestricted access — either the `all`
 * scope, or no scope at all, which the backend treats as a full grant.
 */
export const isUnrestricted = (names: readonly string[]): boolean =>
	names.length === 0 || names.includes("all");
