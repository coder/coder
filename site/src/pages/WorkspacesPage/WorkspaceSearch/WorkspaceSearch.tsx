import type { FC } from "react";
import { SearchField } from "components/Search/SearchField";
import { Stack } from "components/Stack/Stack";
import { PresetFiltersMenu } from "./PresetFiltersMenu";

type WorkspaceSearchProps = {
  query: string;
  setQuery: (query: string) => void;
};

export const WorkspaceSearch: FC<WorkspaceSearchProps> = ({
  query,
  setQuery,
}) => {
  return (
    <Stack alignItems="center" direction="row" spacing={1}>
      <PresetFiltersMenu onSelect={setQuery} />

      <SearchField
        id="search"
        label="Search workspace"
        value={query}
        onChange={setQuery}
      />
    </Stack>
  );
};
