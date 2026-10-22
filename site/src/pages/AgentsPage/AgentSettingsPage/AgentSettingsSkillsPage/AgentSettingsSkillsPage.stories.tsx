import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import {
	organizationSkillPreferencesKey,
	organizationSkills,
} from "#/api/queries/organizationSkills";
import { userSkills } from "#/api/queries/userSkills";
import type { OrganizationSkillPreferences } from "#/api/typesGenerated";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockOrganization3,
	MockUserMember,
} from "#/testHelpers/entities";
import {
	MockDisabledSkill,
	MockOrganizationSkillPreferences,
	MockSkills,
} from "#/testHelpers/skills";
import {
	withAuthProvider,
	withDashboardProvider,
	withToaster,
} from "#/testHelpers/storybook";
import AgentSettingsSkillsPage from "./AgentSettingsSkillsPage";

const meta = {
	title: "pages/AgentsPage/AgentSettingsSkillsPage",
	component: AgentSettingsSkillsPage,
	decorators: [withToaster, withAuthProvider, withDashboardProvider],
	parameters: {
		user: {
			...MockUserMember,
			organization_ids: [
				MockDefaultOrganization.id,
				MockOrganization2.id,
				MockOrganization3.id,
			],
		},
		organizations: [
			MockDefaultOrganization,
			MockOrganization2,
			MockOrganization3,
		],
		reactRouter: reactRouterParameters({
			location: { path: "/agents/settings/skills" },
			routing: { path: "/agents/settings/skills" },
		}),
	},
} satisfies Meta<typeof AgentSettingsSkillsPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const OrganizationSkills: Story = {
	parameters: {
		queries: [
			{
				key: userSkills().queryKey,
				data: MockSkills,
			},
			{
				key: organizationSkills(MockDefaultOrganization.id).queryKey,
				data: [...MockSkills.slice(0, 2), MockDisabledSkill],
			},
			{
				key: organizationSkills(MockOrganization2.id).queryKey,
				data: [MockSkills[2]],
			},
			{
				key: organizationSkills(MockOrganization3.id).queryKey,
				data: [MockDisabledSkill],
			},
			{
				key: organizationSkillPreferencesKey(MockDefaultOrganization.id),
				data: MockOrganizationSkillPreferences,
			},
		],
	},
};

const mockMemberControlsPreferences: OrganizationSkillPreferences = {
	...MockOrganizationSkillPreferences,
	members_can_disable_skills: true,
	disabled_skill_ids: [MockSkills[1].id],
};

export const MemberControls: Story = {
	parameters: {
		queries: [
			{ key: userSkills().queryKey, data: MockSkills },
			{
				key: organizationSkills(MockDefaultOrganization.id).queryKey,
				data: [...MockSkills.slice(0, 2), MockDisabledSkill],
			},
			{
				key: organizationSkills(MockOrganization2.id).queryKey,
				data: [MockSkills[2]],
			},
			{
				key: organizationSkills(MockOrganization3.id).queryKey,
				data: [MockDisabledSkill],
			},
			{
				key: organizationSkillPreferencesKey(MockDefaultOrganization.id),
				data: mockMemberControlsPreferences,
			},
		],
	},
};

const mockAllSkillsOffPreferences: OrganizationSkillPreferences = {
	...mockMemberControlsPreferences,
	all_skills_disabled: true,
};

export const AllOrganizationSkillsTurnedOff: Story = {
	parameters: {
		queries: [
			{ key: userSkills().queryKey, data: MockSkills },
			{
				key: organizationSkills(MockDefaultOrganization.id).queryKey,
				data: [...MockSkills.slice(0, 2), MockDisabledSkill],
			},
			{
				key: organizationSkills(MockOrganization2.id).queryKey,
				data: [MockSkills[2]],
			},
			{
				key: organizationSkills(MockOrganization3.id).queryKey,
				data: [MockDisabledSkill],
			},
			{
				key: organizationSkillPreferencesKey(MockDefaultOrganization.id),
				data: mockAllSkillsOffPreferences,
			},
		],
	},
};

export const OrganizationSkillChoicesError: Story = {
	parameters: {
		queries: [
			{ key: userSkills().queryKey, data: MockSkills },
			{
				key: organizationSkills(MockDefaultOrganization.id).queryKey,
				data: [...MockSkills.slice(0, 2), MockDisabledSkill],
			},
			{
				key: organizationSkills(MockOrganization2.id).queryKey,
				data: [MockSkills[2]],
			},
			{
				key: organizationSkills(MockOrganization3.id).queryKey,
				data: [MockDisabledSkill],
			},
		],
	},
	beforeEach: () => {
		spyOn(
			API.experimental,
			"getOrganizationSkillPreferences",
		).mockRejectedValue(new Error("Failed to load your skill choices."));
	},
};

export const OrganizationSkillChoicesLoading: Story = {
	parameters: {
		queries: [
			{ key: userSkills().queryKey, data: MockSkills },
			{
				key: organizationSkills(MockDefaultOrganization.id).queryKey,
				data: [...MockSkills.slice(0, 2), MockDisabledSkill],
			},
			{
				key: organizationSkills(MockOrganization2.id).queryKey,
				data: [MockSkills[2]],
			},
			{
				key: organizationSkills(MockOrganization3.id).queryKey,
				data: [MockDisabledSkill],
			},
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getOrganizationSkillPreferences").mockReturnValue(
			new Promise(() => {}),
		);
	},
};
