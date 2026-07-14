import { describe, expect, it } from "vitest";
import {
	initialWizardState,
	moduleHasConfigurableVars,
	type TemplateBuilderWizardState,
	toComposeRequest,
	toCreateTemplateRequest,
	type WizardAction,
	wizardReducer,
} from "./wizardState";

function reduce(
	actions: WizardAction[],
	state: TemplateBuilderWizardState = initialWizardState,
): TemplateBuilderWizardState {
	return actions.reduce(wizardReducer, state);
}

describe("wizardReducer", () => {
	describe("SET_BASE", () => {
		it("sets the selected base", () => {
			const state = reduce([
				{
					type: "SET_BASE",
					base: {
						id: "docker",
						name: "Docker",
						hasParameters: false,
						hasPrerequisites: false,
					},
				},
			]);
			expect(state.baseTemplateId).toBe("docker");
			expect(state.selectedBase?.name).toBe("Docker");
		});

		it("clears base variable values when base changes", () => {
			const state = reduce([
				{
					type: "SET_BASE",
					base: {
						id: "docker",
						name: "Docker",
						hasParameters: true,
						hasPrerequisites: false,
					},
				},
				{
					type: "SET_BASE_VARIABLES",
					values: { image: "ubuntu" },
				},
				{
					type: "SET_BASE",
					base: {
						id: "aws-linux",
						name: "AWS Linux",
						hasParameters: true,
						hasPrerequisites: false,
					},
				},
			]);
			expect(state.baseTemplateId).toBe("aws-linux");
			expect(state.baseVariableValues).toEqual({});
		});

		it("preserves base variable values when same base is re-selected", () => {
			const state = reduce([
				{
					type: "SET_BASE",
					base: {
						id: "docker",
						name: "Docker",
						hasParameters: true,
						hasPrerequisites: false,
					},
				},
				{
					type: "SET_BASE_VARIABLES",
					values: { image: "ubuntu" },
				},
				{
					type: "SET_BASE",
					base: {
						id: "docker",
						name: "Docker",
						hasParameters: true,
						hasPrerequisites: false,
					},
				},
			]);
			expect(state.baseVariableValues).toEqual({ image: "ubuntu" });
		});
	});

	describe("SET_BASE_VARIABLES", () => {
		it("replaces all base variable values", () => {
			const state = reduce([
				{
					type: "SET_BASE_VARIABLES",
					values: { image: "ubuntu", region: "us-east-1" },
				},
			]);
			expect(state.baseVariableValues).toEqual({
				image: "ubuntu",
				region: "us-east-1",
			});
		});
	});

	describe("SET_MODULES", () => {
		it("sets modules and metadata", () => {
			const state = reduce([
				{
					type: "SET_MODULES",
					modules: [{ id: "code-server" }],
					meta: [
						{
							id: "code-server",
							name: "code-server",
							iconUrl: "/icon.svg",
							hasConfigurableVars: true,
						},
					],
				},
			]);
			expect(state.modules).toHaveLength(1);
			expect(state.selectedModules).toHaveLength(1);
			expect(state.modules[0].id).toBe("code-server");
		});

		it("preserves existing variable values for still-selected modules", () => {
			const state = reduce([
				{
					type: "SET_MODULES",
					modules: [{ id: "code-server" }],
					meta: [
						{
							id: "code-server",
							name: "code-server",
							iconUrl: "/icon.svg",
							hasConfigurableVars: true,
						},
					],
				},
				{
					type: "SET_MODULE_VARIABLES",
					moduleId: "code-server",
					variables: { port: "13337" },
				},
				{
					type: "SET_MODULES",
					modules: [{ id: "code-server" }, { id: "npm-config" }],
					meta: [
						{
							id: "code-server",
							name: "code-server",
							iconUrl: "/icon.svg",
							hasConfigurableVars: true,
						},
						{
							id: "npm-config",
							name: "npm-config",
							iconUrl: "/npm.svg",
							hasConfigurableVars: false,
						},
					],
				},
			]);
			const codeServer = state.modules.find((m) => m.id === "code-server");
			expect(codeServer?.variables).toEqual({ port: "13337" });
			expect(state.modules).toHaveLength(2);
		});

		it("does not overwrite incoming variables with stale values", () => {
			const state = reduce([
				{
					type: "SET_MODULES",
					modules: [{ id: "code-server" }],
					meta: [
						{
							id: "code-server",
							name: "code-server",
							iconUrl: "/icon.svg",
							hasConfigurableVars: true,
						},
					],
				},
				{
					type: "SET_MODULE_VARIABLES",
					moduleId: "code-server",
					variables: { port: "13337" },
				},
				{
					type: "SET_MODULES",
					modules: [{ id: "code-server", variables: { port: "8080" } }],
					meta: [
						{
							id: "code-server",
							name: "code-server",
							iconUrl: "/icon.svg",
							hasConfigurableVars: true,
						},
					],
				},
			]);
			const codeServer = state.modules.find((m) => m.id === "code-server");
			expect(codeServer?.variables).toEqual({ port: "8080" });
		});
	});

	describe("SET_MODULE_VARIABLES", () => {
		it("updates variables for a specific module", () => {
			const state = reduce([
				{
					type: "SET_MODULES",
					modules: [{ id: "code-server" }, { id: "npm-config" }],
					meta: [
						{
							id: "code-server",
							name: "code-server",
							iconUrl: "/icon.svg",
							hasConfigurableVars: true,
						},
						{
							id: "npm-config",
							name: "npm-config",
							iconUrl: "/npm.svg",
							hasConfigurableVars: true,
						},
					],
				},
				{
					type: "SET_MODULE_VARIABLES",
					moduleId: "code-server",
					variables: { port: "13337" },
				},
			]);
			expect(
				state.modules.find((m) => m.id === "code-server")?.variables,
			).toEqual({ port: "13337" });
			// Other module is unchanged.
			expect(
				state.modules.find((m) => m.id === "npm-config")?.variables,
			).toBeUndefined();
		});
	});

	describe("SET_CUSTOMIZATION", () => {
		it("updates individual customization fields", () => {
			const state = reduce([
				{
					type: "SET_CUSTOMIZATION",
					field: "displayName",
					value: "My Template",
				},
				{
					type: "SET_CUSTOMIZATION",
					field: "description",
					value: "A test template",
				},
				{
					type: "SET_CUSTOMIZATION",
					field: "organizationId",
					value: "org-123",
				},
			]);
			expect(state.displayName).toBe("My Template");
			expect(state.description).toBe("A test template");
			expect(state.organizationId).toBe("org-123");
		});
	});

	describe("RESET", () => {
		it("returns to initial state", () => {
			const state = reduce([
				{
					type: "SET_BASE",
					base: {
						id: "docker",
						name: "Docker",
						hasParameters: true,
						hasPrerequisites: false,
					},
				},
				{
					type: "SET_CUSTOMIZATION",
					field: "displayName",
					value: "My Template",
				},
				{ type: "RESET" },
			]);
			expect(state).toEqual(initialWizardState);
		});
	});
});

describe("moduleHasConfigurableVars", () => {
	it("returns true when module has non-sensitive variables", () => {
		const result = moduleHasConfigurableVars({
			id: "code-server",
			display_name: "code-server",
			description: "",
			icon: "",
			category: "IDE",
			version: "1.0.0",
			compatible_os: ["linux"],
			conflicts_with: [],
			variables: [
				{
					name: "port",
					type: "number",
					description: "Port",
					required: false,
					sensitive: false,
				},
			],
		});
		expect(result).toBe(true);
	});

	it("returns false when all variables are sensitive", () => {
		const result = moduleHasConfigurableVars({
			id: "npm-config",
			display_name: "npm-config",
			description: "",
			icon: "",
			category: "Utility",
			version: "1.0.0",
			compatible_os: ["linux"],
			conflicts_with: [],
			variables: [
				{
					name: "npm_token",
					type: "string",
					description: "Token",
					required: true,
					sensitive: true,
				},
			],
		});
		expect(result).toBe(false);
	});

	it("returns false for empty variables", () => {
		const result = moduleHasConfigurableVars({
			id: "git-clone",
			display_name: "git-clone",
			description: "",
			icon: "",
			category: "Utility",
			version: "1.0.0",
			compatible_os: ["linux"],
			conflicts_with: [],
			variables: [],
		});
		expect(result).toBe(false);
	});
});

describe("toComposeRequest", () => {
	it("produces the correct API request shape", () => {
		const state: TemplateBuilderWizardState = {
			...initialWizardState,
			baseTemplateId: "docker",
			modules: [
				{ id: "code-server", variables: { port: "13337" } },
				{ id: "npm-config" },
			],
		};
		const request = toComposeRequest(state);
		expect(request).toEqual({
			base_template_id: "docker",
			modules: [
				{ id: "code-server", variables: { port: "13337" } },
				{ id: "npm-config" },
			],
		});
	});

	it("uses empty string for base_template_id when no base is selected", () => {
		const request = toComposeRequest(initialWizardState);
		expect(request.base_template_id).toBe("");
		expect(request.modules).toEqual([]);
		expect(request.base_variable_values).toBeUndefined();
	});

	it("includes base_variable_values when set", () => {
		const state: TemplateBuilderWizardState = {
			...initialWizardState,
			baseTemplateId: "kubernetes",
			baseVariableValues: { namespace: "default", use_kubeconfig: "false" },
		};
		const request = toComposeRequest(state);
		expect(request.base_variable_values).toEqual({
			namespace: "default",
			use_kubeconfig: "false",
		});
	});
});

describe("toCreateTemplateRequest", () => {
	it("produces the correct API request shape", () => {
		const state: TemplateBuilderWizardState = {
			...initialWizardState,
			baseTemplateId: "docker",
			organizationId: "org-123",
			name: "my-template",
			displayName: "My Template",
			description: "A test template",
			icon: "/icon/docker.svg",
			modules: [{ id: "code-server" }],
		};
		const request = toCreateTemplateRequest(state);
		expect(request.base_template_id).toBe("docker");
		expect(request.organization_id).toBe("org-123");
		expect(request.name).toBe("my-template");
		expect(request.display_name).toBe("My Template");
		expect(request.description).toBe("A test template");
		expect(request.icon).toBe("/icon/docker.svg");
		expect(request.modules).toEqual([{ id: "code-server" }]);
	});

	it("omits empty optional fields", () => {
		const state: TemplateBuilderWizardState = {
			...initialWizardState,
			baseTemplateId: "docker",
			name: "my-template",
		};
		const request = toCreateTemplateRequest(state);
		expect(request.display_name).toBeUndefined();
		expect(request.description).toBeUndefined();
		expect(request.icon).toBeUndefined();
	});
});
