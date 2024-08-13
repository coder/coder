import type { FC } from "react";
import {
  Filter,
  MenuSkeleton,
  SearchFieldSkeleton,
  type useFilter,
} from "components/Filter/filter";
import { useFilterMenu } from "components/Filter/menu";
import {
  SelectFilter,
  SelectFilterOption,
} from "components/Filter/SelectFilter";
import { API } from "api/api";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { docs } from "utils/docs";
import { Organization } from "api/typesGenerated";

interface TemplatesFilterProps {
  filter: ReturnType<typeof useFilter>;
}

export const TemplatesFilter: FC<TemplatesFilterProps> = ({ filter }) => {
  const organizationMenu = useFilterMenu({
    onChange: (option) =>
      filter.update({ ...filter.values, organization: option?.value }),
    value: filter.values.organization,
    id: "organization",
    getSelectedOption: async () => {
      if (!filter.values.organization) {
        return null;
      }

      const org = await API.getOrganization(filter.values.organization);
      return orgOption(org);
    },
    getOptions: async () => {
      const orgs = await API.getMyOrganizations();
      return orgs.map(orgOption);
    },
  });

  return (
    <Filter
      presets={[
        { query: "", name: "All templates" },
        { query: "deprecated:true", name: "Deprecated templates" },
      ]}
      learnMoreLink={docs("/templates#template-filtering")}
      isLoading={false}
      filter={filter}
      options={
        <>
          <SelectFilter
            placeholder="All organizations"
            label="Select an organization"
            options={organizationMenu.searchOptions}
            selectedOption={organizationMenu.selectedOption ?? undefined}
            onSelect={organizationMenu.selectOption}
          />
        </>
      }
      skeleton={
        <>
          <SearchFieldSkeleton />
          <MenuSkeleton />
        </>
      }
    />
  );
};

const orgOption = (org: Organization): SelectFilterOption => ({
  label: org.display_name || org.name,
  value: org.name,
  startIcon: (
    <UserAvatar
      key={org.id}
      size="sm"
      username={org.display_name}
      avatarURL={org.icon}
    />
  ),
});
