import type { FC } from "react";
import { SearchField } from "components/Search/SearchField";
import { Stack } from "components/Stack/Stack";
import { PresetFiltersMenu } from "./PresetFiltersMenu";
import { StatusMenu } from "./StatusMenu";

type WorkspaceSearchProps = {
  query: string;
  setQuery: (query: string) => void;
};

export const WorkspaceSearch: FC<WorkspaceSearchProps> = ({
  query,
  setQuery,
}) => {
  const status = findTagValue(query, "status");

  return (
    <Stack
      alignItems="center"
      direction="row"
      spacing={1}
      css={{
        "& > *": {
          flexShrink: 0,
        },
      }}
    >
      <PresetFiltersMenu onSelect={setQuery} />

      <SearchField
        id="search"
        label="Search workspace"
        value={query}
        onChange={setQuery}
      />

      <StatusMenu
        selected={status}
        onSelect={(status) => {
          setQuery(replaceOrAddTagValue(query, "status", status));
        }}
      />
    </Stack>
  );
};

function findTagValue(query: string, tag: string): string | undefined {
  const blocks = query.split(" ");
  const block = blocks.find((block) => block.startsWith(`${tag}:`));

  if (!block) {
    return;
  }

  return block.split(":")[1];
}

function replaceOrAddTagValue(
  query: string,
  tag: string,
  value: string,
): string {
  const blocks = query.split(" ");
  const block = blocks.find((block) => block.startsWith(`${tag}:`));

  if (block) {
    return query.replace(block, `${tag}:${value}`);
  }

  return `${query} ${tag}:${value}`;
}
